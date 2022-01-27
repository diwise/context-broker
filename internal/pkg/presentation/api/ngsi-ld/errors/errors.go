package errors

import (
	"encoding/json"
	"net/http"
)

//ProblemDetails stores details about a certain problem according to RFC7807
//See https://tools.ietf.org/html/rfc7807
type ProblemDetails interface {
	ContentType() string
	Type() string
	Title() string
	Detail() string
	MarshalJSON() ([]byte, error)
	WriteResponse(w http.ResponseWriter)
}

//ProblemDetailsImpl is an implementation of the ProblemDetails interface
type ProblemDetailsImpl struct {
	typ    string
	title  string
	detail string
	code   int
}

const (
	//ProblemReportContentType as required by https://tools.ietf.org/html/rfc7807
	ProblemReportContentType string = "application/problem+json"
)

//AlreadyExists reports that the request tries to create an already existing entity
type AlreadyExists struct {
	ProblemDetailsImpl
}

//NewAlreadyExists creates and returns a new instance of an AlreadyExists with the supplied problem detail
func NewAlreadyExists(detail string) *AlreadyExists {
	return &AlreadyExists{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/AlreadyExists",
			title:  "Already Exists",
			detail: detail,
			code:   http.StatusConflict,
		},
	}
}

//ReportNewAlreadyExistsError creates an AlreadyExists instance and sends it to the supplied http.ResponseWriter
func ReportNewAlreadyExistsError(w http.ResponseWriter, detail string) {
	ae := NewAlreadyExists(detail)
	ae.WriteResponse(w)
}

//BadRequestData reports that the request includes input data which does not meet the requirements of the operation
type BadRequestData struct {
	ProblemDetailsImpl
}

//NewBadRequestData creates and returns a new instance of a BadRequestData with the supplied problem detail
func NewBadRequestData(detail string) *BadRequestData {
	return &BadRequestData{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/BadRequestData",
			title:  "Bad Request Data",
			detail: detail,
			code:   http.StatusBadRequest,
		},
	}
}

//ReportNewBadRequestData creates a BadRequestData instance and sends it to the supplied http.ResponseWriter
func ReportNewBadRequestData(w http.ResponseWriter, detail string) {
	brd := NewBadRequestData(detail)
	brd.WriteResponse(w)
}

//InvalidRequest reports that the request associated to the operation is syntactically
//invalid or includes wrong content
type InvalidRequest struct {
	ProblemDetailsImpl
}

//NewInvalidRequest creates and returns a new instance of an InvalidRequest with the supplied problem detail
func NewInvalidRequest(detail string) *InvalidRequest {
	return &InvalidRequest{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/InvalidRequest",
			title:  "Invalid Request",
			detail: detail,
			code:   http.StatusBadRequest,
		},
	}
}

//ReportNewInvalidRequest creates an InvalidRequest instance and sends it to the supplied http.ResponseWriter
func ReportNewInvalidRequest(w http.ResponseWriter, detail string) {
	ir := NewInvalidRequest(detail)
	ir.WriteResponse(w)
}

//InternalError reports that there has been an error during the operation execution
type InternalError struct {
	ProblemDetailsImpl
}

//NewInternalError creates and returns a new instance of an InternalError with the supplied problem detail
func NewInternalError(detail string) *InternalError {
	return &InternalError{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/InternalError",
			title:  "Internal Error",
			detail: detail,
			code:   http.StatusInternalServerError,
		},
	}
}

//ReportNewInternalError creates an InternalError instance and sends it to the supplied http.ResponseWriter
func ReportNewInternalError(w http.ResponseWriter, detail string) {
	ie := NewInternalError(detail)
	ie.WriteResponse(w)
}

//NotFound reports that the request failed with a not found error of some kind
type NotFound struct {
	ProblemDetailsImpl
}

//NewNotFound creates and returns a new instance of a NotFound with the supplied problem detail
func NewNotFound(detail string) *NotFound {
	return &NotFound{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/ResourceNotFound",
			title:  "Not Found",
			detail: detail,
			code:   http.StatusNotFound,
		},
	}
}

//ReportNotFoundError creates a NotFound instance and sends it to the supplied http.ResponseWriter
func ReportNotFoundError(w http.ResponseWriter, detail string) {
	nf := NewNotFound(detail)
	nf.WriteResponse(w)
}

type UnauthorizedRequest struct {
	ProblemDetailsImpl
}

func NewUnauthorizedRequest(detail string) *UnauthorizedRequest {
	return &UnauthorizedRequest{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/UnauthorizedRequest",
			title:  "Unauthorized Request",
			detail: detail,
			code:   http.StatusUnauthorized,
		},
	}
}

func ReportUnauthorizedRequest(w http.ResponseWriter, detail string) {
	ur := NewUnauthorizedRequest(detail)
	ur.WriteResponse(w)
}

//UnknownTenant reports that the request tries to interact with an unknown tenant
type UnknownTenant struct {
	ProblemDetailsImpl
}

//NewUnknownTenant creates and returns a new instance of an UnknownTenant with the supplied problem detail
func NewUnknownTenant(detail string) *UnknownTenant {
	return &UnknownTenant{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:    "https://uri.etsi.org/ngsi-ld/errors/NonexistentTenant",
			title:  "Non Existent Tenant",
			detail: detail,
			code:   http.StatusNotFound,
		},
	}
}

//ReportUnknownTenantError creates an UnknownTenant instance and sends it to the supplied http.ResponseWriter
func ReportUnknownTenantError(w http.ResponseWriter, detail string) {
	ut := NewUnknownTenant(detail)
	ut.WriteResponse(w)
}

//ContentType returns the ContentType to be used when returning this problem
func (p *ProblemDetailsImpl) ContentType() string {
	return ProblemReportContentType
}

//MarshalJSON is called when a ProblemDetailsImpl instance should be serialized to JSON
func (p *ProblemDetailsImpl) MarshalJSON() ([]byte, error) {
	j, err := json.Marshal(struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}{
		Type:   p.typ,
		Title:  p.title,
		Detail: p.detail,
	})
	if err != nil {
		return nil, err
	}
	return j, nil
}

//ResponseCode returns the HTTP response code to be used when returning a specific problem
func (p *ProblemDetailsImpl) ResponseCode() int {

	if p.code != 0 {
		return p.code
	}

	return http.StatusBadRequest
}

//WriteResponse writes the contents of this instance to a http.ResponseWriter
func (p *ProblemDetailsImpl) WriteResponse(w http.ResponseWriter) {
	w.Header().Add("Content-Type", p.ContentType())
	w.Header().Add("Content-Language", "en")
	w.WriteHeader(p.ResponseCode())

	pdbytes, err := json.MarshalIndent(p, "", "  ")
	if err == nil {
		w.Write(pdbytes)
	}
	// else write a 500 error ...
}
