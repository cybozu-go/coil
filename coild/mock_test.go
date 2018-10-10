package coild

type mock struct{}

func newMock() Model {
	return mock{}
}
