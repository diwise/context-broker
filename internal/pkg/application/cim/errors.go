package cim

type AlreadyExistsError struct {
}

func NewAlreadyExistsError() AlreadyExistsError {
	return AlreadyExistsError{}
}

func (aee AlreadyExistsError) Error() string {
	return "already exists"
}
