package cnirpc

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCNI(t *testing.T) {
	st := status.New(codes.Internal, "aaa")
	st, err := st.WithDetails(&CNIError{
		Code:    CNIError_ERR_TRY_AGAIN_LATER,
		Msg:     "abc",
		Details: "detail",
	})
	if err != nil {
		t.Error(err)
	}

	err = st.Err()
	st2, ok := status.FromError(err)
	if !ok {
		t.Error("should get status")
	}

	details := st2.Details()
	if len(details) != 1 {
		t.Fatal(`len(details) != 1`, len(details))
	}

	cniErr, ok := details[0].(*CNIError)
	if !ok {
		t.Fatal(`not a CNIError`)
	}

	if cniErr.Code != CNIError_ERR_TRY_AGAIN_LATER {
		t.Error(`cniErr.Code != CNIError_ERR_TRY_AGAIN_LATER`, cniErr.Code)
	}
}
