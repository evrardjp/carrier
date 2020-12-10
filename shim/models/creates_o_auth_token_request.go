// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// CreatesOAuthTokenRequest creates o auth token request
//
// swagger:model createsOAuthTokenRequest
type CreatesOAuthTokenRequest struct {

	// A unique string representing the registration information provided by the client, the recipient of the token. Optional if it is passed as part of the Basic Authorization header.
	ClientID string `json:"client_id,omitempty"`

	// The secret passphrase configured for the OAuth client. Optional if it is passed as part of the Basic Authorization header.
	ClientSecret string `json:"client_secret,omitempty"`

	// The authorization code, obtained from /oauth/authorize, issued for the user
	// Required: true
	Code *string `json:"code"`

	// The type of authentication being used to obtain the token, in this case authorization_code
	// Required: true
	GrantType *string `json:"grant_type"`

	// Redirection URI to which the authorization server will send the user-agent back once access is granted (or denied)
	RedirectURI string `json:"redirect_uri,omitempty"`

	// Can be set to opaque to retrieve an opaque and revocable token or to jwt to retrieve a JWT token. If not set the zone setting config.tokenPolicy.jwtRevocable is used.
	TokenFormat string `json:"token_format,omitempty"`
}

// Validate validates this creates o auth token request
func (m *CreatesOAuthTokenRequest) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateCode(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateGrantType(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *CreatesOAuthTokenRequest) validateCode(formats strfmt.Registry) error {

	if err := validate.Required("code", "body", m.Code); err != nil {
		return err
	}

	return nil
}

func (m *CreatesOAuthTokenRequest) validateGrantType(formats strfmt.Registry) error {

	if err := validate.Required("grant_type", "body", m.GrantType); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *CreatesOAuthTokenRequest) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *CreatesOAuthTokenRequest) UnmarshalBinary(b []byte) error {
	var res CreatesOAuthTokenRequest
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
