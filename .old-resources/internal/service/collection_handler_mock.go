// Code generated by MockGen. DO NOT EDIT.
// Source: collection_handler.go
//
// Generated by this command:
//
//	mockgen -source=collection_handler.go -package=service -destination=collection_handler_mock.go
//
// Package service is a generated GoMock package.
package service

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockCollectionHandler is a mock of CollectionHandler interface.
type MockCollectionHandler struct {
	ctrl     *gomock.Controller
	recorder *MockCollectionHandlerMockRecorder
}

// MockCollectionHandlerMockRecorder is the mock recorder for MockCollectionHandler.
type MockCollectionHandlerMockRecorder struct {
	mock *MockCollectionHandler
}

// NewMockCollectionHandler creates a new mock instance.
func NewMockCollectionHandler(ctrl *gomock.Controller) *MockCollectionHandler {
	mock := &MockCollectionHandler{ctrl: ctrl}
	mock.recorder = &MockCollectionHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCollectionHandler) EXPECT() *MockCollectionHandlerMockRecorder {
	return m.recorder
}

// List mocks base method.
func (m *MockCollectionHandler) List(ctx context.Context, request *ListRequest) (*ListResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List", ctx, request)
	ret0, _ := ret[0].(*ListResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// List indicates an expected call of List.
func (mr *MockCollectionHandlerMockRecorder) List(ctx, request any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "List", reflect.TypeOf((*MockCollectionHandler)(nil).List), ctx, request)
}