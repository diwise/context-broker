package cim

import "fmt"

type AlreadyExistsError struct {
}

func NewAlreadyExistsError() AlreadyExistsError {
	return AlreadyExistsError{}
}

func (aee AlreadyExistsError) Error() string {
	return "already exists"
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
