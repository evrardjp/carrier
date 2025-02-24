package v1

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/epinio/epinio/helpers/kubernetes"
	"github.com/epinio/epinio/internal/application"
	"github.com/epinio/epinio/internal/organizations"
	"github.com/epinio/epinio/internal/services"
	"github.com/epinio/epinio/pkg/api/core/v1/models"
	"github.com/julienschmidt/httprouter"
)

// ServicesController represents all functionality of the API related to services
type ServicesController struct {
}

// Show handles the API end point /orgs/:org/services/:service
// It returns the detail information of the named service instance
func (sc ServicesController) Show(w http.ResponseWriter, r *http.Request) APIErrors {
	ctx := r.Context()
	params := httprouter.ParamsFromContext(ctx)
	org := params.ByName("org")
	serviceName := params.ByName("service")

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return InternalError(err)
	}

	exists, err := organizations.Exists(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}
	if !exists {
		return OrgIsNotKnown(org)
	}

	service, err := services.Lookup(ctx, cluster, org, serviceName)
	if err != nil {
		if err.Error() == "service not found" {
			return ServiceIsNotKnown(serviceName)
		}
		if err != nil {
			return InternalError(err)
		}
	}

	serviceDetails, err := service.Details(ctx)
	if err != nil {
		return InternalError(err)
	}

	responseData := map[string]string{}
	for key, value := range serviceDetails {
		responseData[key] = value
	}

	err = jsonResponse(w, models.ServiceShowResponse{
		Username: service.User(),
		Details:  responseData,
	})
	if err != nil {
		return InternalError(err)
	}

	return nil
}

// Index handles the API end point /orgs/:org/services
// It returns a list of all known service instances
func (sc ServicesController) Index(w http.ResponseWriter, r *http.Request) APIErrors {
	ctx := r.Context()
	params := httprouter.ParamsFromContext(ctx)
	org := params.ByName("org")

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return InternalError(err)
	}

	exists, err := organizations.Exists(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}
	if !exists {
		return OrgIsNotKnown(org)
	}

	orgServices, err := services.List(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}

	appsOf, err := servicesToApps(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}

	var responseData models.ServiceResponseList

	for _, service := range orgServices {
		var appNames []string

		for _, app := range appsOf[service.Name()] {
			appNames = append(appNames, app.Meta.Name)
		}
		responseData = append(responseData, models.ServiceResponse{
			Name:      service.Name(),
			BoundApps: appNames,
		})
	}

	err = jsonResponse(w, responseData)
	if err != nil {
		return InternalError(err)
	}

	return nil
}

// Create handles the API end point /orgs/:org/services
// It creates the named service from its parameters
func (sc ServicesController) Create(w http.ResponseWriter, r *http.Request) APIErrors {
	ctx := r.Context()
	params := httprouter.ParamsFromContext(ctx)
	org := params.ByName("org")
	username, err := GetUsername(r)
	if err != nil {
		return UserNotFound()
	}

	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return InternalError(err)
	}

	var createRequest models.ServiceCreateRequest
	err = json.Unmarshal(bodyBytes, &createRequest)
	if err != nil {
		return BadRequest(err)
	}

	if createRequest.Name == "" {
		return NewBadRequest("Cannot create service without a name")
	}

	if len(createRequest.Data) < 1 {
		return NewBadRequest("Cannot create service without data")
	}

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return InternalError(err)
	}

	exists, err := organizations.Exists(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}
	if !exists {
		return OrgIsNotKnown(org)
	}

	// Verify that the requested name is not yet used by a different service.
	_, err = services.Lookup(ctx, cluster, org, createRequest.Name)
	if err == nil {
		// no error, service is found, conflict
		return ServiceAlreadyKnown(createRequest.Name)
	}
	if err != nil && err.Error() != "service not found" {
		// some internal error
		return InternalError(err)
	}
	// any error here is `service not found`, and we can continue

	// Create the new service. At last.
	_, err = services.CreateService(ctx, cluster, createRequest.Name, org, username, createRequest.Data)
	if err != nil {
		return InternalError(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	err = jsonResponse(w, models.ResponseOK)
	if err != nil {
		return InternalError(err)
	}

	return nil
}

// Delete handles the API end point /orgs/:org/services/:service (DELETE)
// It deletes the named service
func (sc ServicesController) Delete(w http.ResponseWriter, r *http.Request) APIErrors {
	ctx := r.Context()
	params := httprouter.ParamsFromContext(ctx)
	org := params.ByName("org")
	serviceName := params.ByName("service")
	username, err := GetUsername(r)
	if err != nil {
		return UserNotFound()
	}

	defer r.Body.Close()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return InternalError(err)
	}

	var deleteRequest models.ServiceDeleteRequest
	err = json.Unmarshal(bodyBytes, &deleteRequest)
	if err != nil {
		return BadRequest(err)
	}

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return InternalError(err)
	}

	exists, err := organizations.Exists(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}
	if !exists {
		return OrgIsNotKnown(org)
	}

	service, err := services.Lookup(ctx, cluster, org, serviceName)
	if err != nil && err.Error() == "service not found" {
		return ServiceIsNotKnown(serviceName)
	}
	if err != nil {
		return InternalError(err)
	}

	// Verify that the service is unbound. IOW not bound to any application.
	// If it is, and automatic unbind was requested, do that.
	// Without automatic unbind such applications are reported as error.

	boundAppNames := []string{}
	appsOf, err := servicesToApps(ctx, cluster, org)
	if err != nil {
		return InternalError(err)
	}
	if boundApps, found := appsOf[service.Name()]; found {
		for _, app := range boundApps {
			boundAppNames = append(boundAppNames, app.Meta.Name)
		}

		if !deleteRequest.Unbind {
			return NewBadRequest("bound applications exist", strings.Join(boundAppNames, ","))
		}

		for _, app := range boundApps {
			apiErr := DeleteBinding(ctx, cluster, org, app.Meta.Name, serviceName, username)
			if apiErr != nil {
				return apiErr
			}
		}
	}

	// Everything looks to be ok. Delete.

	err = service.Delete(ctx)
	if err != nil {
		return InternalError(err)
	}

	err = jsonResponse(w, models.ServiceDeleteResponse{BoundApps: boundAppNames})
	if err != nil {
		return InternalError(err)
	}

	return nil
}

// servicesToApps is a helper to Index and Delete. It produces a map
// from service instances names to application names, the apps bound
// to each service.
func servicesToApps(ctx context.Context, cluster *kubernetes.Cluster, org string) (map[string]models.AppList, error) {
	// Determine apps bound to services
	// (inversion of services bound to apps)
	// Literally query apps in the org for their services and invert.

	var appsOf = map[string]models.AppList{}

	apps, err := application.List(ctx, cluster, org)
	if err != nil {
		return nil, err
	}

	for _, app := range apps {
		for _, bound := range app.Configuration.Services {
			if theapps, found := appsOf[bound]; found {
				appsOf[bound] = append(theapps, app)
			} else {
				appsOf[bound] = models.AppList{app}
			}
		}
	}

	return appsOf, nil
}
