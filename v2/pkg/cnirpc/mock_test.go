package cnirpc

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type mockServer struct {
	UnimplementedCNIServer

	sockName string
}

func (m *mockServer) Check(context.Context, *CNIArgs) (*emptypb.Empty, error) {
	st := status.New(codes.Internal, "aaa")
	st, err := st.WithDetails(&CNIError{
		Code:    ErrorCode_TRY_AGAIN_LATER,
		Msg:     "abc",
		Details: "detail",
	})
	if err != nil {
		panic(err)
	}

	return nil, st.Err()
}

func (m *mockServer) Start(ctx context.Context) error {
	f, err := os.CreateTemp("", "coild-mock")
	if err != nil {
		return err
	}
	sockName := f.Name()
	f.Close()
	os.Remove(sockName)

	s, err := net.Listen("unix", sockName)
	if err != nil {
		return err
	}
	m.sockName = sockName
	grpcServer := grpc.NewServer()
	RegisterCNIServer(grpcServer, m)

	go func() {
		err := grpcServer.Serve(s)
		if err != nil {
			panic(err)
		}
	}()
	go func(ctx context.Context) {
		<-ctx.Done()
		grpcServer.Stop()
		os.Remove(sockName)
	}(ctx)

	return nil
}

func TestCNIWithMock(t *testing.T) {
	s := &mockServer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
	}()

	err := s.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dialer := &net.Dialer{}
	dialFunc := func(ctx context.Context, a string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", a)
	}
	conn, err := grpc.Dial(s.sockName, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialFunc))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := NewCNIClient(conn)
	_, err = client.Check(context.Background(), &CNIArgs{})
	if err == nil {
		t.Fatal("err is expected")
	}
	t.Log(err)

	st := status.Convert(err)
	if st.Code() != codes.Internal {
		t.Error(`st.Code() != codes.Internal`)
	}
	if st.Message() != "aaa" {
		t.Error(`st.Message() != "aaa"`)
	}

	details := st.Details()
	if len(details) != 1 {
		t.Fatal(`len(details) != 1`, len(details))
	}

	cniErr, ok := details[0].(*CNIError)
	if !ok {
		t.Fatal(`not a CNIError`)
	}

	if cniErr.Code != ErrorCode_TRY_AGAIN_LATER {
		t.Error(`cniErr.Code != CNIError_ERR_TRY_AGAIN_LATER`, cniErr.Code)
	}
}
