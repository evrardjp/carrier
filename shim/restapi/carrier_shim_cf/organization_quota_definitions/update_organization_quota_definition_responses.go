// Code generated by go-swagger; DO NOT EDIT.

package organization_quota_definitions

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/suse/carrier/shim/models"
)

// UpdateOrganizationQuotaDefinitionCreatedCode is the HTTP code returned for type UpdateOrganizationQuotaDefinitionCreated
const UpdateOrganizationQuotaDefinitionCreatedCode int = 201

/*UpdateOrganizationQuotaDefinitionCreated successful response

swagger:response updateOrganizationQuotaDefinitionCreated
*/
type UpdateOrganizationQuotaDefinitionCreated struct {

	/*
	  In: Body
	*/
	Payload *models.UpdateOrganizationQuotaDefinitionResponseResource `json:"body,omitempty"`
}

// NewUpdateOrganizationQuotaDefinitionCreated creates UpdateOrganizationQuotaDefinitionCreated with default headers values
func NewUpdateOrganizationQuotaDefinitionCreated() *UpdateOrganizationQuotaDefinitionCreated {

	return &UpdateOrganizationQuotaDefinitionCreated{}
}

// WithPayload adds the payload to the update organization quota definition created response
func (o *UpdateOrganizationQuotaDefinitionCreated) WithPayload(payload *models.UpdateOrganizationQuotaDefinitionResponseResource) *UpdateOrganizationQuotaDefinitionCreated {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the update organization quota definition created response
func (o *UpdateOrganizationQuotaDefinitionCreated) SetPayload(payload *models.UpdateOrganizationQuotaDefinitionResponseResource) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *UpdateOrganizationQuotaDefinitionCreated) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(201)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
