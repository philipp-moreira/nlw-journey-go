package mailpit

import (
	"context"
	"fmt"
	"journey/cmd/journey/config"
	"journey/internal/pgstore"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wneessen/go-mail"
)

type store interface {
	GetTrip(context.Context, uuid.UUID) (pgstore.Trip, error)
}

type Mailpit struct {
	store store
}

func NewMailPit(pool *pgxpool.Pool) Mailpit {
	return Mailpit{pgstore.New(pool)}
}

func (mp Mailpit) SendConfirmTripEmailToTripOwner(tripId uuid.UUID) error {
	ctx := context.Background()
	trip, err := mp.store.GetTrip(ctx, tripId)
	if err != nil {
		return fmt.Errorf("mailpit: failed to get trip for SendConfirmTripEmailToTripOwner: %w", err)
	}

	msg := mail.NewMsg()
	if err := msg.From("oi@planner.com"); err != nil {
		return fmt.Errorf("mailpit: failed to set 'From' in email SendConfirmTripEmailToTripOwner: %w", err)
	}

	if err := msg.To(trip.OwnerEmail); err != nil {
		return fmt.Errorf("mailpit: failed to set 'to' in email SendConfirmTripEmailToTripOwner: %w", err)
	}

	portApp, err := getPortApplication("SendConfirmTripEmailToTripOwner")
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://localhost:%v/trips/%v/confirm", portApp, trip.ID.String())
	msg.Subject(fmt.Sprintf("Confirme sua presença na viagem para %v em %v", trip.Destination, trip.StartsAt.Time.Format(time.DateOnly)))
	msg.SetBodyString(mail.TypeTextHTML, fmt.Sprintf(`
        <div style="font-family: sans-serif; font-size: 16px; line-height: 1.6;">
          <p>Você solicitou a criação de uma viagem para <strong>%v</strong> nas datas de <strong>%v</strong> até <strong>%v</strong>.</p>
          <p></p>
          <p>Para confirmar sua viagem, clique no link abaixo:</p>
          <p></p>
          <p>
            <a href="%v">Confirmar viagem</a>
          </p>
          <p></p>
          <p>Caso você não saiba do que se trata esse e-mail, apenas ignore esse e-mail.</p>
        </div>
		`,
		trip.Destination, trip.StartsAt.Time.Format(time.DateOnly), trip.EndsAt.Time.Format(time.DateOnly), url,
	))

	client, err := mail.NewClient("mailpit", mail.WithTLSPortPolicy(mail.NoTLS), mail.WithPort(1025))
	if err != nil {
		return fmt.Errorf("mailpit: failed create email client SendConfirmTripEmailToTripOwner: %w", err)
	}

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("mailpit: failed send email client SendConfirmTripEmailToTripOwner: %w", err)
	}

	return nil
}

func (mp Mailpit) SendConfirmTripEmailToParticipants(data SendInviteToParticipants) error {

	msg := mail.NewMsg()
	if err := msg.From("mailpit@journey.com"); err != nil {
		return fmt.Errorf("mailpit: failed to set 'From' in email SendConfirmTripEmailToParticipants: %w", err)
	}

	portApp, err := getPortApplication("SendConfirmTripEmailToTripOwner")
	if err != nil {
		return err
	}

	for _, invite := range data.Invites {

		if err := msg.To(invite.Participant.Email); err != nil {
			return fmt.Errorf("mailpit: failed to set 'to' in email SendConfirmTripEmailToParticipants: %w", err)
		}

		url := fmt.Sprintf("http://localhost:%v/participants/%v/confirm", portApp, invite.Participant.ParticipantId)
		msg.Subject("Confirme sua viagem")
		msg.SetBodyString(mail.TypeTextHTML, fmt.Sprintf(`
		<div style="font-family: sans-serif; font-size: 16px; line-height: 1.6;">
		  <p>Você foi convidado(a) para participar de uma viagem para <strong>%v</strong> nas datas de <strong>%v</strong> até <strong>%v</strong>.</p>
		  <p></p>
		  <p>Para confirmar sua presença na viagem, clique no link abaixo:</p>
		  <p></p>
		  <p>
			<a href="%v">Confirmar viagem</a>
		  </p>
		  <p></p>
		  <p>Caso você não saiba do que se trata esse e-mail, apenas ignore esse e-mail.</p>
		</div>
	`,
			data.Trip.Destination, data.Trip.StartsAt.Time.Format(time.DateOnly), data.Trip.EndsAt.Time.Format(time.DateOnly), url,
		))

		client, err := mail.NewClient("mailpit", mail.WithTLSPortPolicy(mail.NoTLS), mail.WithPort(1025))
		if err != nil {
			return fmt.Errorf("mailpit: failed to set 'to' in email SendConfirmTripEmailToParticipants: %w", err)
		}

		if err := client.DialAndSend(msg); err != nil {
			return fmt.Errorf("mailpit: failed to set 'to' in email SendConfirmTripEmailToParticipants: %w", err)
		}
	}

	return nil
}

func getPortApplication(nameFunctionCaller string) (string, error) {
	stringEmpty := ""

	port, err := config.GetSpecificEnvironmentVariable("JOURNEY_APP_PORT")
	if err != nil {
		return stringEmpty, fmt.Errorf("don't possible get port to application on send e-mail confirmation in '%s'", nameFunctionCaller)
	}

	return port, nil
}

type SendInviteToParticipants struct {
	Trip    pgstore.Trip
	Invites []InviteParticipantsToTrip
}

type InviteParticipantsToTrip struct {
	TripID      uuid.UUID
	Participant Participant
}

type Participant struct {
	Email         string
	ParticipantId uuid.UUID
}
