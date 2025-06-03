package register_feed_usecase

import (
	"alt/mocks"
	"alt/port/register_feed_port"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRegisterFeedUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRegisterFeedPort := mocks.NewMockRegisterFeedPort(ctrl)

	type fields struct {
		registerFeedGateway register_feed_port.RegisterFeedPort
	}
	type args struct {
		ctx  context.Context
		link string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				registerFeedGateway: mockRegisterFeedPort,
			},
			args: args{
				ctx:  context.Background(),
				link: "https://example.com/rss",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegisterFeedPort.EXPECT().RegisterRSSFeedLink(tt.args.ctx, tt.args.link).Return(nil).Times(1)

			r := NewRegisterFeedUsecase(tt.fields.registerFeedGateway)
			if err := r.Execute(tt.args.ctx, tt.args.link); (err != nil) != tt.wantErr {
				t.Errorf("RegisterFeedUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
