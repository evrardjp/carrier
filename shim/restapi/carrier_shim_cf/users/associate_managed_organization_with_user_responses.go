// Code generated by go-swagger; DO NOT EDIT.

package users

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/suse/carrier/shim/models"
)

// AssociateManagedOrganizationWithUserCreatedCode is the HTTP code returned for type AssociateManagedOrganizationWithUserCreated
const AssociateManagedOrganizationWithUserCreatedCode int = 201

/*AssociateManagedOrganizationWithUserCreated successful response

swagger:response associateManagedOrganizationWithUserCreated
*/
type AssociateManagedOrganizationWithUserCreated struct {

	/*
	  In: Body
	*/
	Payload *models.AssociateManagedOrganizationWithUserResponseResource `json:"body,omitempty"`
}

// NewAssociateManagedOrganizationWithUserCreated creates AssociateManagedOrganizationWithUserCreated with default headers values
func NewAssociateManagedOrganizationWithUserCreated() *AssociateManagedOrganizationWithUserCreated {

	return &AssociateManagedOrganizationWithUserCreated{}
}

// WithPayload adds the payload to the associate managed organization with user created response
func (o *AssociateManagedOrganizationWithUserCreated) WithPayload(payload *models.AssociateManagedOrganizationWithUserResponseResource) *AssociateManagedOrganizationWithUserCreated {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the associate managed organization with user created response
func (o *AssociateManagedOrganizationWithUserCreated) SetPayload(payload *models.AssociateManagedOrganizationWithUserResponseResource) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *AssociateManagedOrganizationWithUserCreated) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(201)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
