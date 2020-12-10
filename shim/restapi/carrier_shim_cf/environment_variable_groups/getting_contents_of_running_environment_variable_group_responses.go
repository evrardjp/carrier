// Code generated by go-swagger; DO NOT EDIT.

package environment_variable_groups

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/suse/carrier/shim/models"
)

// GettingContentsOfRunningEnvironmentVariableGroupOKCode is the HTTP code returned for type GettingContentsOfRunningEnvironmentVariableGroupOK
const GettingContentsOfRunningEnvironmentVariableGroupOKCode int = 200

/*GettingContentsOfRunningEnvironmentVariableGroupOK successful response

swagger:response gettingContentsOfRunningEnvironmentVariableGroupOK
*/
type GettingContentsOfRunningEnvironmentVariableGroupOK struct {

	/*
	  In: Body
	*/
	Payload *models.GettingContentsOfRunningEnvironmentVariableGroupResponseResource `json:"body,omitempty"`
}

// NewGettingContentsOfRunningEnvironmentVariableGroupOK creates GettingContentsOfRunningEnvironmentVariableGroupOK with default headers values
func NewGettingContentsOfRunningEnvironmentVariableGroupOK() *GettingContentsOfRunningEnvironmentVariableGroupOK {

	return &GettingContentsOfRunningEnvironmentVariableGroupOK{}
}

// WithPayload adds the payload to the getting contents of running environment variable group o k response
func (o *GettingContentsOfRunningEnvironmentVariableGroupOK) WithPayload(payload *models.GettingContentsOfRunningEnvironmentVariableGroupResponseResource) *GettingContentsOfRunningEnvironmentVariableGroupOK {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the getting contents of running environment variable group o k response
func (o *GettingContentsOfRunningEnvironmentVariableGroupOK) SetPayload(payload *models.GettingContentsOfRunningEnvironmentVariableGroupResponseResource) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *GettingContentsOfRunningEnvironmentVariableGroupOK) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(200)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
