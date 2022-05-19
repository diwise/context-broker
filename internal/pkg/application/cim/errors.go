package cim

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type AlreadyExistsError struct {
	msg string
}

func NewAlreadyExistsError(msg string) AlreadyExistsError {
	return AlreadyExistsError{msg: msg}
}

func (aee AlreadyExistsError) Error() string {
	return aee.msg
}

type BadRequestDataError struct {
	msg string
}

func NewBadRequestDataError(msg string) BadRequestDataError {
	return BadRequestDataError{msg: msg}
}

func (brd BadRequestDataError) Error() string {
	return brd.msg
}

type InvalidRequestError struct {
	msg string
}

func NewInvalidRequestError(msg string) InvalidRequestError {
	return InvalidRequestError{msg: msg}
}

func (ire InvalidRequestError) Error() string {
	return ire.msg
}

type NotFoundError struct {
	msg string
}

func NewNotFoundError(msg string) NotFoundError {
	return NotFoundError{msg: msg}
}

func (nfe NotFoundError) Error() string {
	return nfe.msg
}

type UnknownTenantError struct {
	tenant string
}

func NewUnknownTenantError(tenant string) UnknownTenantError {
	return UnknownTenantError{tenant: tenant}
}

func (ute UnknownTenantError) Error() string {
	return fmt.Sprintf("unknown tenant \"%s\"", ute.tenant)
}

// TODO: Move problem report handling to a single place (presentation layer)

func NewErrorFromProblemReport(code int, contentType string, body []byte) error {
	report := &struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}{}

	err := json.Unmarshal(body, report)
	if err != nil {
		return fmt.Errorf("failed to process problem report from context source: %s", err.Error())
	}

	if code == http.StatusNotFound || report.Type == "https://uri.etsi.org/ngsi-ld/errors/ResourceNotFound" {
		return NewNotFoundError(report.Detail)
	}

	if report.Type == "https://uri.etsi.org/ngsi-ld/errors/BadRequestData" {
		return NewBadRequestDataError(report.Detail)
	}

	if report.Type == "https://uri.etsi.org/ngsi-ld/errors/InvalidRequest" {
		return NewInvalidRequestError(report.Detail)
	}

	if report.Type == "https://uri.etsi.org/ngsi-ld/errors/AlreadyExists" {
		return NewAlreadyExistsError(report.Detail)
	}

	return fmt.Errorf("[error: %d] unknown problem report of type \"%s\" with detail \"%s\" received", code, report.Type, report.Detail)
}
