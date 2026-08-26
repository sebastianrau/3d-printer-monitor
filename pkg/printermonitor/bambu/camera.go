package bambu

import "context"

// Camera provides a model-independent Bambu snapshot interface. Concrete
// implementations own the transport and video format used by a printer family.
type Camera interface {
	CaptureSnapshot(context.Context) ([]byte, error)
}
