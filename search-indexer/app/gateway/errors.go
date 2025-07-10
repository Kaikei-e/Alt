package gateway

type DriverError struct {
	Op  string
	Err string
}

func (e *DriverError) Error() string {
	return e.Op + ": " + e.Err
}
