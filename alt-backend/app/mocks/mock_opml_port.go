// Code generated manually for OPML port mocks.
package mocks

import (
	domain "alt/domain"
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockExportOPMLPort is a mock of ExportOPMLPort interface.
type MockExportOPMLPort struct {
	ctrl     *gomock.Controller
	recorder *MockExportOPMLPortMockRecorder
	isgomock struct{}
}

// MockExportOPMLPortMockRecorder is the mock recorder for MockExportOPMLPort.
type MockExportOPMLPortMockRecorder struct {
	mock *MockExportOPMLPort
}

// NewMockExportOPMLPort creates a new mock instance.
func NewMockExportOPMLPort(ctrl *gomock.Controller) *MockExportOPMLPort {
	mock := &MockExportOPMLPort{ctrl: ctrl}
	mock.recorder = &MockExportOPMLPortMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockExportOPMLPort) EXPECT() *MockExportOPMLPortMockRecorder {
	return m.recorder
}

// FetchFeedLinksForExport mocks base method.
func (m *MockExportOPMLPort) FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchFeedLinksForExport", ctx)
	ret0, _ := ret[0].([]*domain.FeedLinkForExport)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchFeedLinksForExport indicates an expected call of FetchFeedLinksForExport.
func (mr *MockExportOPMLPortMockRecorder) FetchFeedLinksForExport(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchFeedLinksForExport", reflect.TypeOf((*MockExportOPMLPort)(nil).FetchFeedLinksForExport), ctx)
}

// MockImportOPMLPort is a mock of ImportOPMLPort interface.
type MockImportOPMLPort struct {
	ctrl     *gomock.Controller
	recorder *MockImportOPMLPortMockRecorder
	isgomock struct{}
}

// MockImportOPMLPortMockRecorder is the mock recorder for MockImportOPMLPort.
type MockImportOPMLPortMockRecorder struct {
	mock *MockImportOPMLPort
}

// NewMockImportOPMLPort creates a new mock instance.
func NewMockImportOPMLPort(ctrl *gomock.Controller) *MockImportOPMLPort {
	mock := &MockImportOPMLPort{ctrl: ctrl}
	mock.recorder = &MockImportOPMLPortMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockImportOPMLPort) EXPECT() *MockImportOPMLPortMockRecorder {
	return m.recorder
}

// RegisterFeedLinkBulk mocks base method.
func (m *MockImportOPMLPort) RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterFeedLinkBulk", ctx, urls)
	ret0, _ := ret[0].(*domain.OPMLImportResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterFeedLinkBulk indicates an expected call of RegisterFeedLinkBulk.
func (mr *MockImportOPMLPortMockRecorder) RegisterFeedLinkBulk(ctx, urls any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterFeedLinkBulk", reflect.TypeOf((*MockImportOPMLPort)(nil).RegisterFeedLinkBulk), ctx, urls)
}
