package koi

// Validator is an interface for validating input.
type Validator interface {
	Validate() error
}

// Statuser is an interface for customizing the status code of an error.
type Statuser interface {
	Status() int
}
