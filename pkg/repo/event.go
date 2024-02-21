package repo

import (
	"context"
	"database/sql"
	"errors"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/repo/model"
	"github.com/mylxsw/eloquent/query"
	"gopkg.in/guregu/null.v3"
)

const (
	// EventTypeUserCreated is the event type for user created
	EventTypeUserCreated = "user_created"
	// EventTypeUserPhoneBound is the event type for user phone bound
	EventTypeUserPhoneBound = "user_phone_bound"
	// EventTypePaymentCompleted is the event type for payment completed
	EventTypePaymentCompleted = "payment_completed"
)

const (
	// EventStatusWaiting is the status for waiting event
	EventStatusWaiting = "waiting"
	// EventStatusSucceed is the status for succeed event
	EventStatusSucceed = "succeed"
	// EventStatusFailed is the status for failed event
	EventStatusFailed = "failed"
)

// UserCreatedEvent is the event for user created
type UserCreatedEvent struct {
	UserID int64                  `json:"user_id"`
	From   UserCreatedEventSource `json:"from"`
}

type UserCreatedEventSource string

const (
	UserCreatedEventSourceEmail  UserCreatedEventSource = "email"
	UserCreatedEventSourcePhone  UserCreatedEventSource = "phone"
	UserCreatedEventSourceWechat UserCreatedEventSource = "wechat"
)

type UserBindEvent struct {
	UserID int64  `json:"user_id"`
	Phone  string `json:"phone"`
}

type PaymentCompletedEvent struct {
	UserID    int64  `json:"user_id"`
	ProductID string `json:"product_id"`
	PaymentID string `json:"payment_id"`
}

type EventRepo struct {
	db   *sql.DB
	conf *config.Config
}

func NewEventRepo(db *sql.DB, conf *config.Config) *EventRepo {
	return &EventRepo{db: db, conf: conf}
}

// GetEvent get event by id
func (repo *EventRepo) GetEvent(ctx context.Context, id int64) (*model.Events, error) {
	event, err := model.NewEventsModel(repo.db).First(ctx, query.Builder().Where(model.FieldEventsId, id))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	ret := event.ToEvents()
	return &ret, nil
}

// UpdateEvent update event status
func (repo *EventRepo) UpdateEvent(ctx context.Context, id int64, status string) error {
	_, err := model.NewEventsModel(repo.db).Update(ctx, query.Builder().Where(model.FieldEventsId, id), model.EventsN{
		Status: null.StringFrom(status),
	})

	return err
}
