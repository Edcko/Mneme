package vector

import "fmt"

type ErrInvalidBlob struct {
	Reason   string
	Declared int
	Actual   int
}

func (e ErrInvalidBlob) Error() string {
	if e.Declared > 0 {
		return fmt.Sprintf("invalid embedding blob: %s (declared n=%d, actual size=%d)", e.Reason, e.Declared, e.Actual)
	}
	return fmt.Sprintf("invalid embedding blob: %s", e.Reason)
}
