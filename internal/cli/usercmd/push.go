package usercmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/epinio/epinio/helpers"
	"github.com/epinio/epinio/internal/duration"
	"github.com/epinio/epinio/pkg/api/core/v1/models"
)

type PushParams struct {
	Configuration models.ApplicationUpdateRequest // instances, services, EVs
	Docker        string
	GitRev        string
	BuilderImage  string
	Name          string
	Path          string
}

// Push pushes an app
// * validate
// * upload
// * stage
// * (tail logs)
// * wait for pipelinerun
// * deploy
// * wait for app
func (c *EpinioClient) Push(ctx context.Context, params PushParams) error {
	name := params.Name
	source := params.Path

	appRef := models.AppRef{Name: name, Org: c.Config.Org}
	log := c.Log.
		WithName("Push").
		WithValues("Name", appRef.Name,
			"Namespace", appRef.Org,
			"Sources", source)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute. Visible via TRACE_LEVEL=2

	sourceToShow := source
	if params.GitRev != "" {
		sourceToShow = fmt.Sprintf("%s @ %s", sourceToShow, params.GitRev)
	}

	msg := c.ui.Note().
		WithStringValue("Name", appRef.Name).
		WithStringValue("Sources", sourceToShow).
		WithStringValue("Namespace", appRef.Org)

	if err := c.TargetOk(); err != nil {
		return err
	}

	if len(params.Configuration.Services) > 0 {
		msg = msg.WithStringValue("Services:",
			strings.Join(params.Configuration.Services, ", "))
	}

	msg.Msg("About to push an application with given name and sources into the specified namespace")

	c.ui.Exclamation().
		Timeout(duration.UserAbort()).
		Msg("Hit Enter to continue or Ctrl+C to abort (deployment will continue automatically in 5 seconds)")

	details.Info("validate app name")
	errorMsgs := validation.IsDNS1123Subdomain(appRef.Name)
	if len(errorMsgs) > 0 {
		return fmt.Errorf("%s: %s", "app name incorrect", strings.Join(errorMsgs, "\n"))
	}

	// AppCreate
	c.ui.Normal().Msg("Create the application resource ...")

	request := models.ApplicationCreateRequest{
		Name:          appRef.Name,
		Configuration: params.Configuration,
	}

	_, err := c.API.AppCreate(request, appRef.Org)
	if err != nil {
		// try to recover if it's a response type Conflict error and not a http connection error
		rerr, ok := err.(interface{ StatusCode() int })

		if !ok {
			return err
		}

		if rerr.StatusCode() != http.StatusConflict {
			return err
		}

		c.ui.Normal().Msg("Application exists, updating ...")
		details.Info("app exists conflict")

		_, err := c.API.AppUpdate(params.Configuration, appRef.Org, appRef.Name)
		if err != nil {
			return err
		}
	}

	// AppUpload / AppImportGit
	var blobUID string
	if params.GitRev == "" && params.Docker == "" {
		c.ui.Normal().Msg("Collecting the application sources ...")

		tmpDir, tarball, err := helpers.Tar(source)
		defer func() {
			if tmpDir != "" {
				_ = os.RemoveAll(tmpDir)
			}
		}()
		if err != nil {
			return err
		}

		c.ui.Normal().Msg("Uploading application code ...")

		details.Info("upload code")
		upload, err := c.API.AppUpload(appRef.Org, appRef.Name, tarball)
		if err != nil {
			return err
		}
		log.V(3).Info("upload response", "response", upload)

		blobUID = upload.BlobUID

	} else if params.GitRev != "" {
		c.ui.Normal().Msg("Importing the application sources from Git ...")

		gitRef := models.GitRef{
			URL:      source,
			Revision: params.GitRev,
		}
		response, err := c.API.AppImportGit(appRef, gitRef)
		if err != nil {
			return errors.Wrap(err, "importing git remote")
		}
		blobUID = response.BlobUID
	}

	// AppStage
	stageID := ""
	var stageResponse *models.StageResponse
	if params.Docker == "" {
		c.ui.Normal().Msg("Staging application via docker image ...")

		req := models.StageRequest{
			App:          appRef,
			BlobUID:      blobUID,
			BuilderImage: params.BuilderImage,
		}
		details.Info("staging code", "Blob", blobUID)
		stageResponse, err = c.API.AppStage(req)
		if err != nil {
			return err
		}
		stageID = stageResponse.Stage.ID
		log.V(3).Info("stage response", "response", stageResponse)

		details.Info("start tailing logs", "StageID", stageResponse.Stage.ID)
		err = c.stageLogs(details, appRef, stageResponse.Stage.ID)
		if err != nil {
			return err
		}
	}

	// AppDeploy
	c.ui.Normal().Msg("Deploying application ...")
	deployRequest := models.DeployRequest{
		App: appRef,
	}
	// If docker param is specified, then we just take it into ImageURL
	// If not, we take the one from the staging response
	if params.Docker != "" {
		deployRequest.ImageURL = params.Docker
	} else {
		deployRequest.ImageURL = stageResponse.ImageURL
		deployRequest.Stage = models.StageRef{ID: stageID}
	}

	deployResponse, err := c.API.AppDeploy(deployRequest)
	if err != nil {
		return err
	}

	details.Info("wait for application resources")
	c.ui.ProgressNote().KeeplineUnder(1).Msg("Creating application resources")

	_, err = c.API.AppRunning(appRef)
	if err != nil {
		return errors.Wrap(err, "waiting for app failed")
	}

	c.ui.Success().
		WithStringValue("Name", appRef.Name).
		WithStringValue("Namespace", appRef.Org).
		WithStringValue("Route", fmt.Sprintf("https://%s", deployResponse.Route)).
		WithStringValue("Builder Image", params.BuilderImage).
		Msg("App is online.")

	return nil
}

func (c *EpinioClient) stageLogs(details logr.Logger, appRef models.AppRef, stageID string) error {
	// Buffered because the go routine may no longer be listening when we try
	// to stop it. Stopping it should be a fire and forget. We have wg to wait
	// for the routine to be gone.
	stopChan := make(chan bool, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		err := c.AppLogs(appRef.Name, stageID, true, stopChan)
		if err != nil {
			c.ui.Problem().Msg(fmt.Sprintf("failed to tail logs: %s", err.Error()))
		}
	}()

	details.Info("wait for pipelinerun", "StageID", stageID)
	c.ui.ProgressNote().KeeplineUnder(1).Msg("Running staging")

	_, err := c.API.StagingComplete(appRef.Org, stageID)
	if err != nil {
		stopChan <- true // Stop the printing go routine
		return errors.Wrap(err, "waiting for staging failed")
	}
	stopChan <- true // Stop the printing go routine

	return err
}
