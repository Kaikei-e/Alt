package test_utils

// Multi-server test environment
type TestEnvironment struct {
	RSSServer      *MockRSSServer
	DatabaseServer *MockDatabaseServer
	SearchServer   *MockSearchServer
}

func NewTestEnvironment() *TestEnvironment {
	return &TestEnvironment{
		RSSServer:      NewMockRSSServer(),
		DatabaseServer: NewMockDatabaseServer(),
		SearchServer:   NewMockSearchServer(),
	}
}

func (te *TestEnvironment) Close() {
	if te.RSSServer != nil {
		te.RSSServer.Close()
	}
	if te.DatabaseServer != nil {
		te.DatabaseServer.Close()
	}
	if te.SearchServer != nil {
		te.SearchServer.Close()
	}
}
