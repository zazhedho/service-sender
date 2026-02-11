package interfaceemail

import (
	"context"

	"service-sender/internal/dto"
)

type ServiceEmailInterface interface {
	Send(ctx context.Context, req dto.SendEmailRequest, appName string) (int, string, error)
}
