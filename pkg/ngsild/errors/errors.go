package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var ErrAlreadyExists = fmt.Errorf("already exists")
var ErrInternal = fmt.Errorf("internal error")
var ErrNotFound = fmt.Errorf("not found")
var ErrRequest = fmt.Errorf("request error")
var ErrBadRequest = fmt.Errorf("bad request")
var ErrBadResponse = fmt.Errorf("bad response")
var ErrInvalidRequest = fmt.Errorf("invalid request")
var ErrUnknownTenant = fmt.Errorf("unknown tenant")

type myError struct {
	msg    string
	target error
}

func (m myError) Error() string        { return m.msg }
func (m myError) Is(target error) bool { return target == m.target }

func NewAlreadyExistsError(msg string) error {
	return &myError{
		msg:    msg,
		target: ErrAlreadyExists,
	}
}

func NewBadRequestDataError(msg string) error {
	return &myError{
		msg:    msg,
		target: ErrBadRequest,
	}
}

func NewInvalidRequestError(msg string) error {
	return &myError{
		msg:    msg,
		target: ErrInvalidRequest,
	}
}

func NewNotFoundError(msg string) error {
	return &myError{
		msg:    msg,
		target: ErrNotFound,
	}
}

func NewUnknownTenantError(msg string) error {
	return &myError{
		msg:    msg,
		target: ErrUnknownTenant,
	}
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

	if report.Type == "https://uri.etsi.org/ngsi-ld/errors/NonexistentTenant" {
		return NewUnknownTenantError(report.Detail)
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

	return NewInternalError(
		fmt.Sprintf("[code: %d] unknown problem report of type \"%s\" with detail \"%s\" received",
			code, report.Type, report.Detail,
		),
		"traceID",
	)
}

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
	typ     string
	title   string
	detail  string
	code    int
	traceID string
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
func NewAlreadyExists(detail, traceID string) *AlreadyExists {
	return &AlreadyExists{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/AlreadyExists",
			title:   "Already Exists",
			detail:  detail,
			code:    http.StatusConflict,
			traceID: traceID,
		},
	}
}

//ReportNewAlreadyExistsError creates an AlreadyExists instance and sends it to the supplied http.ResponseWriter
func ReportNewAlreadyExistsError(w http.ResponseWriter, detail, traceID string) {
	ae := NewAlreadyExists(detail, traceID)
	ae.WriteResponse(w)
}

//BadRequestData reports that the request includes input data which does not meet the requirements of the operation
type BadRequestData struct {
	ProblemDetailsImpl
}

//NewBadRequestData creates and returns a new instance of a BadRequestData with the supplied problem detail
func NewBadRequestData(detail, traceID string) *BadRequestData {
	return &BadRequestData{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/BadRequestData",
			title:   "Bad Request Data",
			detail:  detail,
			code:    http.StatusBadRequest,
			traceID: traceID,
		},
	}
}

//ReportNewBadRequestData creates a BadRequestData instance and sends it to the supplied http.ResponseWriter
func ReportNewBadRequestData(w http.ResponseWriter, detail, traceID string) {
	brd := NewBadRequestData(detail, traceID)
	brd.WriteResponse(w)
}

//InvalidRequest reports that the request associated to the operation is syntactically
//invalid or includes wrong content
type InvalidRequest struct {
	ProblemDetailsImpl
}

//NewInvalidRequest creates and returns a new instance of an InvalidRequest with the supplied problem detail
func NewInvalidRequest(detail, traceID string) *InvalidRequest {
	return &InvalidRequest{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/InvalidRequest",
			title:   "Invalid Request",
			detail:  detail,
			code:    http.StatusBadRequest,
			traceID: traceID,
		},
	}
}

//ReportNewInvalidRequest creates an InvalidRequest instance and sends it to the supplied http.ResponseWriter
func ReportNewInvalidRequest(w http.ResponseWriter, detail, traceID string) {
	ir := NewInvalidRequest(detail, traceID)
	ir.WriteResponse(w)
}

//InternalError reports that there has been an error during the operation execution
type InternalError struct {
	ProblemDetailsImpl
}

func (ie InternalError) Error() string {
	return ie.detail
}

//NewInternalError creates and returns a new instance of an InternalError with the supplied problem detail
func NewInternalError(detail, traceID string) *InternalError {
	return &InternalError{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/InternalError",
			title:   "Internal Error",
			detail:  detail,
			code:    http.StatusInternalServerError,
			traceID: traceID,
		},
	}
}

//ReportNewInternalError creates an InternalError instance and sends it to the supplied http.ResponseWriter
func ReportNewInternalError(w http.ResponseWriter, detail, traceID string) {
	ie := NewInternalError(detail, traceID)
	ie.WriteResponse(w)
}

//NotFound reports that the request failed with a not found error of some kind
type NotFound struct {
	ProblemDetailsImpl
}

//NewNotFound creates and returns a new instance of a NotFound with the supplied problem detail
func NewNotFound(detail, traceID string) *NotFound {
	return &NotFound{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/ResourceNotFound",
			title:   "Not Found",
			detail:  detail,
			code:    http.StatusNotFound,
			traceID: traceID,
		},
	}
}

//ReportNotFoundError creates a NotFound instance and sends it to the supplied http.ResponseWriter
func ReportNotFoundError(w http.ResponseWriter, detail, traceID string) {
	nf := NewNotFound(detail, traceID)
	nf.WriteResponse(w)
}

type UnauthorizedRequest struct {
	ProblemDetailsImpl
}

func NewUnauthorizedRequest(detail, traceID string) *UnauthorizedRequest {
	return &UnauthorizedRequest{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/UnauthorizedRequest",
			title:   "Unauthorized Request",
			detail:  detail,
			code:    http.StatusUnauthorized,
			traceID: traceID,
		},
	}
}

func ReportUnauthorizedRequest(w http.ResponseWriter, detail, traceID string) {
	ur := NewUnauthorizedRequest(detail, traceID)
	ur.WriteResponse(w)
}

//UnknownTenant reports that the request tries to interact with an unknown tenant
type UnknownTenant struct {
	ProblemDetailsImpl
}

//NewUnknownTenant creates and returns a new instance of an UnknownTenant with the supplied problem detail
func NewUnknownTenant(detail, traceID string) *UnknownTenant {
	return &UnknownTenant{
		ProblemDetailsImpl: ProblemDetailsImpl{
			typ:     "https://uri.etsi.org/ngsi-ld/errors/NonexistentTenant",
			title:   "Non Existent Tenant",
			detail:  detail,
			code:    http.StatusNotFound,
			traceID: traceID,
		},
	}
}

//ReportUnknownTenantError creates an UnknownTenant instance and sends it to the supplied http.ResponseWriter
func ReportUnknownTenantError(w http.ResponseWriter, detail, traceID string) {
	ut := NewUnknownTenant(detail, traceID)
	ut.WriteResponse(w)
}

//ContentType returns the ContentType to be used when returning this problem
func (p *ProblemDetailsImpl) ContentType() string {
	return ProblemReportContentType
}

//MarshalJSON is called when a ProblemDetailsImpl instance should be serialized to JSON
func (p *ProblemDetailsImpl) MarshalJSON() ([]byte, error) {
	var traceID *string

	if p.traceID != "" {
		traceID = &p.traceID
	}

	j, err := json.Marshal(struct {
		Type    string  `json:"type"`
		Title   string  `json:"title"`
		Detail  string  `json:"detail"`
		TraceID *string `json:"traceID,omitempty"`
	}{
		Type:    p.typ,
		Title:   p.title,
		Detail:  p.detail,
		TraceID: traceID,
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
