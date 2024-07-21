package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"journey/internal/api/spec"
	"journey/internal/mailer/mailpit"
	"journey/internal/pgstore"
	"net/http"
	"strings"
	"time"

	"github.com/discord-gophers/goapi-gen/types"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/payfazz/baseurl"
	"go.uber.org/zap"
)

type mailer interface {
	SendConfirmTripEmailToTripOwner(uuid.UUID) error
	SendConfirmTripEmailToParticipants(mailpit.SendInviteToParticipants) error
}

type store interface {
	// Trips
	CreateTrip(context.Context, *pgxpool.Pool, spec.CreateTripRequest) (uuid.UUID, error)
	GetTrip(context.Context, uuid.UUID) (pgstore.Trip, error)
	UpdateTrip(context.Context, pgstore.UpdateTripParams) error
	UpdateTripConfirm(context.Context, pgstore.UpdateTripConfirmParams) error
	// Participants
	ConfirmParticipant(context.Context, pgstore.ConfirmParticipantParams) error
	GetParticipant(context.Context, uuid.UUID) (pgstore.Participant, error)
	GetParticipants(context.Context, uuid.UUID) ([]pgstore.Participant, error)
	InviteParticipantsToTrip(context.Context, []pgstore.InviteParticipantsToTripParams) (int64, error)
	// Activities
	CreateActivity(context.Context, pgstore.CreateActivityParams) (uuid.UUID, error)
	GetTripActivities(context.Context, uuid.UUID) ([]pgstore.Activity, error)
	// Links
	CreateTripLink(context.Context, pgstore.CreateTripLinkParams) (uuid.UUID, error)
	GetTripLinks(context.Context, uuid.UUID) ([]pgstore.Link, error)
}

type API struct {
	store     store
	logger    *zap.Logger
	validator *validator.Validate
	pool      *pgxpool.Pool
	mailer    mailer
}

func NewApi(pool *pgxpool.Pool, logger *zap.Logger, mailer mailer) API {
	validator := validator.New(validator.WithRequiredStructEnabled())
	return API{
		pgstore.New(pool),
		logger,
		validator,
		pool,
		mailer,
	}
}

// Create a new trip
// (POST /trips)
func (api *API) PostTrips(w http.ResponseWriter, r *http.Request) *spec.Response {

	var body spec.CreateTripRequest
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		spec.PostTripsJSON400Response(spec.BadRequest{Message: "invalid request: " + err.Error()})
	}

	if err := api.validator.Struct(body); err != nil {
		return spec.PostTripsJSON400Response(spec.BadRequest{Message: "invalid input: " + err.Error()})
	}

	if body.StartsAt.UTC().Before(time.Now().UTC()) {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{Message: "the travel period is invalid, it is not possible to change the start date to before today/now"})
	}

	if body.EndsAt.UTC().Before(body.StartsAt.UTC()) {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{Message: "the travel period is invalid, end date must be equal to or greater than the start date"})
	}

	tripID, err := api.store.CreateTrip(r.Context(), api.pool, body)
	if err != nil {
		api.logger.Error(
			fmt.Sprintf("failed route: '%v: %v' when create a trip: ", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
		)

		return spec.PostTripsJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to create trip, contact adm",
		})
	}

	go func() {
		if err := api.mailer.SendConfirmTripEmailToTripOwner(tripID); err != nil {
			api.logger.Error(
				"failed to send email on PostTrips",
				zap.Error(err),
				zap.String("trip_id", tripID.String()),
			)
		}
	}()

	return spec.PostTripsJSON201Response(spec.CreateTripResponse{TripID: tripID.String()})
}

// Wrapper to confirm a trip and send e-mail invitations.
// (GET /trips/{tripId}/confirm)
func (api *API) GetTripsTripIDConfirm(w http.ResponseWriter, r *http.Request, tripId string) *spec.Response {

	response, err := api.buildRedirectRequestUsingRequestsWithParametersInTheURL(r, r.RequestURI)
	if err != nil {
		api.logger.Error(
			fmt.Sprintf("failed on route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripId", tripId),
		)

		return spec.GetTripsTripIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to confirm trip by wrapper",
		})
	}

	if response.StatusCode == 400 {
		var body400 spec.BadRequest
		json.NewDecoder(response.Body).Decode(&body400)
		return spec.GetTripsTripIDConfirmJSON400Response(body400)
	}

	if response.StatusCode == 404 {
		var body404 spec.NotFoundRequest
		json.NewDecoder(response.Body).Decode(&body404)
		return spec.GetTripsTripIDConfirmJSON404Response(body404)
	}

	return spec.GetTripsTripIDConfirmJSON204Response(response.Body)
}

// Confirm a trip and send e-mail invitations.
// (PATCH /trips/{tripId}/confirm)
func (api *API) PatchTripsTripIDConfirm(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyErrorMessage, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.PatchTripsTripIDConfirmJSON400Response(spec.BadRequest{
			Message: friendlyErrorMessage,
		})
	}

	trip, err := api.store.GetTrip(r.Context(), tripUUID)
	if err != nil {
		return spec.PatchTripsTripIDConfirmJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	confirmTrip := pgstore.UpdateTripConfirmParams{
		IsConfirmed: true,
		ID:          tripUUID,
	}

	if err := api.store.UpdateTripConfirm(r.Context(), confirmTrip); err != nil {

		api.logger.Error(
			fmt.Sprintf("failed route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripID", tripID),
		)

		return spec.PatchTripsTripIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to confirm trip and send notifications",
		})
	}

	participants, err := api.store.GetParticipants(r.Context(), tripUUID)
	if err != nil {
		return spec.PatchTripsTripIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to get participants to invite",
		})
	}

	invites := make([]mailpit.InviteParticipantsToTrip, len(participants))
	for index, participant := range participants {
		invites[index] = mailpit.InviteParticipantsToTrip{
			TripID: trip.ID,
			Participant: mailpit.Participant{
				ParticipantId: participant.ID,
				Email:         participant.Email,
			},
		}
	}

	dataToSendInvite := mailpit.SendInviteToParticipants{
		Trip:    trip,
		Invites: invites,
	}

	go func() {
		if err := api.mailer.SendConfirmTripEmailToParticipants(dataToSendInvite); err != nil {
			api.logger.Error(
				"failed to send email on GetTripsTripIDConfirm",
				zap.Error(err),
				zap.String("tripID", tripID),
			)
		}
	}()

	return spec.PatchTripsTripIDConfirmJSON204Response(nil)
}

// Wrapper to confirms a participant on a trip.
// (GET /participants/{participantId}/confirm)
func (api *API) GetParticipantsParticipantIDConfirm(w http.ResponseWriter, r *http.Request, participantID string) *spec.Response {

	response, err := api.buildRedirectRequestUsingRequestsWithParametersInTheURL(r, r.RequestURI)
	if err != nil {
		api.logger.Error(
			fmt.Sprintf("failed on route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripId", participantID),
		)

		return spec.GetParticipantsParticipantIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to confirm participant by wrapper",
		})
	}

	if response.StatusCode == 400 {
		var body400 spec.BadRequest
		json.NewDecoder(response.Body).Decode(&body400)
		return spec.GetParticipantsParticipantIDConfirmJSON400Response(body400)
	}

	if response.StatusCode == 404 {
		var body404 spec.NotFoundRequest
		json.NewDecoder(response.Body).Decode(&body404)
		return spec.GetParticipantsParticipantIDConfirmJSON404Response(body404)
	}

	return spec.GetParticipantsParticipantIDConfirmJSON204Response(response.Body)
}

// Confirms a participant on a trip.
// (PATCH /participants/{participantId}/confirm)
func (api *API) PatchParticipantsParticipantIDConfirm(w http.ResponseWriter, r *http.Request, participantID string) *spec.Response {
	participantUUID, friendlyMessageError, err := api.tryParseUUID("participantID", participantID)
	if err != nil {
		return spec.PatchParticipantsParticipantIDConfirmJSON400Response(spec.BadRequest{
			Message: friendlyMessageError,
		})
	}

	participant, err := api.store.GetParticipant(r.Context(), participantUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spec.PatchParticipantsParticipantIDConfirmJSON404Response(spec.NotFoundRequest{
				Message: "participant not found",
			})
		}

		api.logger.Error(
			fmt.Sprintf("failed on route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("participantID", participantID),
		)

		return spec.PatchParticipantsParticipantIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to retrieve trip's participants",
		})
	}

	if participant.IsConfirmed {
		return spec.PatchParticipantsParticipantIDConfirmJSON400Response(spec.BadRequest{
			Message: "participant already confirmed",
		})
	}

	confirmParticipant := pgstore.ConfirmParticipantParams{
		IsConfirmed: true,
		ID:          participantUUID,
	}

	if err := api.store.ConfirmParticipant(r.Context(), confirmParticipant); err != nil {

		api.logger.Error(
			fmt.Sprintf("failed route: ''%v: %v'' when updating confirmation: ", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("participantID", participantID),
		)

		return spec.PatchParticipantsParticipantIDConfirmJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to retrieve trip's participants",
		})
	}

	return spec.PatchParticipantsParticipantIDConfirmJSON204Response(nil)
}

// Get a trip participants.
// (GET /trips/{tripId}/participants)
func (api *API) GetTripsTripIDParticipants(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyErrorMessage, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.GetTripsTripIDParticipantsJSON400Response(spec.BadRequest{
			Message: friendlyErrorMessage,
		})
	}

	if _, err := api.store.GetTrip(r.Context(), tripUUID); err != nil {
		return spec.GetTripsTripIDParticipantsJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	participants, err := api.store.GetParticipants(r.Context(), tripUUID)
	if err != nil {
		api.logger.Error(
			fmt.Sprintf("failed on route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripID", tripUUID.String()),
		)
		return spec.GetTripsTripIDParticipantsJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to retrieve trip's participants",
		})
	}

	participantsParsed := make([]spec.GetTripParticipantsResponseArray, len(participants))
	for index := 0; index < len(participants); index++ {
		participant := participants[index]
		participantsParsed[index] = spec.GetTripParticipantsResponseArray{
			ID:          participant.ID.String(),
			Email:       types.Email(participant.Email),
			IsConfirmed: participant.IsConfirmed,
		}
	}

	return spec.GetTripsTripIDParticipantsJSON200Response(spec.GetTripParticipantsResponse{
		Participants: participantsParsed,
	})
}

// Get a trip details.
// (GET /trips/{tripId})
func (api *API) GetTripsTripID(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyMessageError, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.GetTripsTripIDJSON400Response(spec.BadRequest{
			Message: friendlyMessageError,
		})
	}

	tripDetail, err := api.store.GetTrip(r.Context(), tripUUID)
	if err != nil {
		return spec.GetTripsTripIDJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	// TODO: Verificar como garantir a geracao do spec da API garantindo a ordenacao mais amigavel das propriedades
	return spec.GetTripsTripIDJSON200Response(spec.GetTripDetailsResponse{
		Trip: spec.GetTripDetailsResponseTripObj{
			ID:          tripDetail.ID.String(),
			Destination: tripDetail.Destination,
			StartsAt:    tripDetail.StartsAt.Time,
			EndsAt:      tripDetail.EndsAt.Time,
			IsConfirmed: tripDetail.IsConfirmed,
		}},
	)
}

// Update a trip.
// (PUT /trips/{tripId})
func (api *API) PutTripsTripID(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyMessageError, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{
			Message: friendlyMessageError,
		})
	}

	var body spec.PutTripsTripIDJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{Message: "json body request invalid. " + err.Error()})
	}

	if err := api.validator.Struct(body); err != nil {
		return spec.PostTripsJSON400Response(spec.BadRequest{Message: "json body request invalid. " + err.Error()})
	}

	tripActual, err := api.store.GetTrip(r.Context(), tripUUID)
	if err != nil {
		return spec.PutTripsTripIDJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	activitiesFromActualTrip, err := api.store.GetTripActivities(r.Context(), tripUUID)
	if err != nil {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{
			Message: "unable to apply consistence, before update, " + err.Error(),
		})
	}

	if body.StartsAt.UTC().Before(time.Now().UTC()) {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{Message: "the travel period is invalid, it is not possible to change the start date to before today/now"})
	}

	if body.EndsAt.UTC().Before(body.StartsAt.UTC()) {
		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{Message: "the travel period is invalid, end date must be equal to or greater than the start date"})
	}

	activitiesOutFromChangesInTrip := api.filterActivities(activitiesFromActualTrip, func(activity pgstore.Activity) bool {
		return body.StartsAt.After(activity.OccursAt.Time) || body.EndsAt.Before(activity.OccursAt.Time)
	})

	if len(activitiesOutFromChangesInTrip) > 0 {
		activitiesId := make([]string, len(activitiesOutFromChangesInTrip))
		for index := 0; index < len(activitiesOutFromChangesInTrip); index++ {
			activitiesId[index] = activitiesOutFromChangesInTrip[index].ID.String()
		}

		return spec.PutTripsTripIDJSON400Response(spec.BadRequest{
			Message: "changes invalid. There are activities occuring out of range the new period's trip. Activities out of range: " + strings.Join(activitiesId, ", "),
		})
	}

	var trip = pgstore.UpdateTripParams{
		Destination: body.Destination,
		EndsAt:      pgtype.Timestamp{Valid: true, Time: body.EndsAt},
		StartsAt:    pgtype.Timestamp{Valid: true, Time: body.StartsAt},
		IsConfirmed: tripActual.IsConfirmed,
		ID:          tripActual.ID,
	}

	if err := api.store.UpdateTrip(r.Context(), trip); err != nil {

		api.logger.Error(
			fmt.Sprintf("failed route: '%v: %v' when updating trip: ", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripID", tripID),
		)

		return spec.PutTripsTripIDJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to update trip",
		})
	}

	return spec.PutTripsTripIDJSON204Response(nil)
}

// Get a trip activities.
// (GET /trips/{tripId}/activities)
func (api *API) GetTripsTripIDActivities(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripIdConverted, friendlyMessageError, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.GetTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: friendlyMessageError,
		})
	}

	trip, err := api.store.GetTrip(r.Context(), tripIdConverted)
	if err != nil {
		return spec.GetTripsTripIDActivitiesJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	activities, err := api.store.GetTripActivities(r.Context(), tripIdConverted)
	if err != nil {

		api.logger.Error(
			fmt.Sprintf("failed route: '%v: %v'", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripID", tripID),
		)

		return spec.GetTripsTripIDActivitiesJSON500Response(spec.InternalServerErrorRequest{
			Message: "anything wrong to get activities",
		})
	}

	numberOfDaysOfTheTrip := ((int)(trip.EndsAt.Time.Sub(trip.StartsAt.Time).Hours()/24) + 1)
	tripDays := make([]time.Time, numberOfDaysOfTheTrip)
	activitiesParsedToResponse := make([]spec.GetTripActivitiesResponseOuterArray, numberOfDaysOfTheTrip)

	for index := 0; index < numberOfDaysOfTheTrip; index++ {
		year, month, day := trip.StartsAt.Time.AddDate(0, 0, index).Date()
		tripDays[index] = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}

	for indexTripDays := 0; indexTripDays < len(tripDays); indexTripDays++ {

		tripDay := tripDays[indexTripDays]

		activitiesFiltered := api.filterActivities(activities, func(activity pgstore.Activity) bool {
			return activity.OccursAt.Time.Truncate(24 * time.Hour).Equal(tripDay.Truncate(24 * time.Hour))
		})

		activitiesFilteredParsed := make([]spec.GetTripActivitiesResponseInnerArray, len(activitiesFiltered))

		for indexActivitiesFiltered := 0; indexActivitiesFiltered < len(activitiesFiltered); indexActivitiesFiltered++ {
			activitiesFilteredParsed[indexActivitiesFiltered] = spec.GetTripActivitiesResponseInnerArray{
				ID:       activitiesFiltered[indexActivitiesFiltered].ID.String(),
				Title:    activitiesFiltered[indexActivitiesFiltered].Title,
				OccursAt: activitiesFiltered[indexActivitiesFiltered].OccursAt.Time,
			}
		}

		activitiesParsedToResponse[indexTripDays] = spec.GetTripActivitiesResponseOuterArray{
			Date:       tripDay,
			Activities: activitiesFilteredParsed,
		}
	}

	return spec.GetTripsTripIDActivitiesJSON200Response(spec.GetTripActivitiesResponse{
		Activities: activitiesParsedToResponse,
	})
}

// Create a trip activity.
// (POST /trips/{tripId}/activities)
func (api *API) PostTripsTripIDActivities(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripIdConverted, friendlyMessageError, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: friendlyMessageError,
		})
	}

	var body spec.PostTripsTripIDActivitiesJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: "invalid request: " + err.Error(),
		})
	}

	if err := api.validator.Struct(body); err != nil {
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: "invalid request: " + err.Error(),
		})
	}

	trip, err := api.store.GetTrip(r.Context(), tripIdConverted)
	if err != nil {
		return spec.PostTripsTripIDActivitiesJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	if body.OccursAt.UTC().Before(trip.StartsAt.Time.UTC()) || body.OccursAt.UTC().After(trip.EndsAt.Time.UTC()) {
		message := fmt.Sprintf("invalid activity,  date of occurrence outside the travel periods ( '%s' to '%s')", trip.StartsAt.Time, trip.EndsAt.Time)
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: message,
		})
	}

	activity := pgstore.CreateActivityParams{
		TripID:   tripIdConverted,
		Title:    body.Title,
		OccursAt: pgtype.Timestamp{Valid: true, Time: body.OccursAt},
	}

	activityId, err := api.store.CreateActivity(r.Context(), activity)
	if err != nil {

		api.logger.Error(
			fmt.Sprintf("failed route: '%v: %v' when create a activitie: ", r.URL.RawPath, r.URL.Path),
			zap.Error(err),
			zap.String("tripID", tripID),
		)

		return spec.PostTripsTripIDActivitiesJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to create activity, contact adm",
		})
	}

	return spec.PostTripsTripIDActivitiesJSON201Response(spec.CreateActivityResponse{ActivityID: activityId.String()})
}

// Invite someone to the trip.
// (POST /trips/{tripId}/invites)
func (api *API) PostTripsTripIDInvites(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyErrorMessage, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.PostTripsTripIDInvitesJSON400Response(spec.BadRequest{
			Message: friendlyErrorMessage,
		})
	}

	var body spec.PostTripsTripIDInvitesJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: "invalid request: " + err.Error(),
		})
	}

	if err := api.validator.Struct(body); err != nil {
		return spec.PostTripsTripIDActivitiesJSON400Response(spec.BadRequest{
			Message: "invalid request: " + err.Error(),
		})
	}

	trip, err := api.store.GetTrip(r.Context(), tripUUID)
	if err != nil {
		return spec.PostTripsTripIDInvitesJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	participants, err := api.store.GetParticipants(r.Context(), tripUUID)
	if err != nil {
		return spec.PostTripsTripIDInvitesJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to obtain participants and consists of whether the new participant sent already exists",
		})
	}

	participantsAlreadyExists := api.filterParticipants(participants, func(participant pgstore.Participant) bool {
		return strings.TrimSpace(participant.Email) == strings.TrimSpace(string(body.Email))
	})

	if len(participantsAlreadyExists) > 0 {
		return spec.PostTripsTripIDInvitesJSON400Response(spec.BadRequest{
			Message: "new participant already exists",
		})
	}

	invitesToInsert := make([]pgstore.InviteParticipantsToTripParams, 1)
	invitesToInsert[0] = pgstore.InviteParticipantsToTripParams{
		TripID: trip.ID,
		Email:  string(body.Email),
	}

	if _, err := api.store.InviteParticipantsToTrip(r.Context(), invitesToInsert); err != nil {
		return spec.PostTripsTripIDInvitesJSON400Response(spec.BadRequest{
			Message: "unable to insert new participant",
		})
	}

	participants, err = api.store.GetParticipants(r.Context(), tripUUID)
	if err != nil {
		return spec.PostTripsTripIDInvitesJSON400Response(spec.BadRequest{
			Message: "new participant registered, but don't was possible recovery operation id",
		})
	}

	participantsNoninvited := api.filterParticipants(participants, func(participant pgstore.Participant) bool {
		return !participant.IsConfirmed
	})

	var participantId uuid.UUID
	for _, participant := range participants {
		if participant.Email == string(body.Email) {
			participantId = participant.ID
			break
		}
	}

	invitesToSend := make([]mailpit.InviteParticipantsToTrip, len(participantsNoninvited))
	for index, participantToInvite := range participantsNoninvited {
		invite := mailpit.InviteParticipantsToTrip{
			TripID: tripUUID,
			Participant: mailpit.Participant{
				ParticipantId: participantToInvite.ID,
				Email:         participantToInvite.Email,
			},
		}
		invitesToSend[index] = invite
	}

	dataToSendInvite := mailpit.SendInviteToParticipants{
		Trip:    trip,
		Invites: invitesToSend,
	}

	go func() {
		if err := api.mailer.SendConfirmTripEmailToParticipants(dataToSendInvite); err != nil {
			api.logger.Error(
				"failed to send email on PostTripsTripIDInvites",
				zap.Error(err),
				zap.String("tripID", tripID),
			)
		}
	}()

	return spec.PostTripsTripIDInvitesJSON201Response(spec.InviteParticipantResponse{
		ParticipantID: participantId.String(),
	})
}

// Get a trip links.
// (GET /trips/{tripId}/links)
func (api *API) GetTripsTripIDLinks(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyErrorMessage, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.GetTripsTripIDLinksJSON400Response(spec.BadRequest{
			Message: friendlyErrorMessage,
		})
	}

	if _, err := api.store.GetTrip(r.Context(), tripUUID); err != nil {
		return spec.GetTripsTripIDLinksJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	links, err := api.store.GetTripLinks(r.Context(), tripUUID)
	if err != nil {
		return spec.GetTripsTripIDLinksJSON400Response(spec.BadRequest{
			Message: "unable to get link to trip",
		})
	}

	linksParsed := make([]spec.GetLinksResponseArray, len(links))
	for index := 0; index < len(links); index++ {
		link := links[index]
		linksParsed[index] = spec.GetLinksResponseArray{
			ID:    link.ID.String(),
			Title: link.Title,
			URL:   link.Url,
		}
	}

	return spec.GetTripsTripIDLinksJSON200Response(spec.GetLinksResponse{
		Links: linksParsed,
	})
}

// Create a trip link.
// (POST /trips/{tripId}/links)
func (api *API) PostTripsTripIDLinks(w http.ResponseWriter, r *http.Request, tripID string) *spec.Response {
	tripUUID, friendlyErrorMessage, err := api.tryParseUUID("tripID", tripID)
	if err != nil {
		return spec.PostTripsTripIDLinksJSON400Response(spec.BadRequest{
			Message: friendlyErrorMessage,
		})
	}

	var body spec.CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return spec.PostTripsTripIDLinksJSON400Response(spec.BadRequest{
			Message: "request invalid " + err.Error(),
		})
	}

	if err := api.validator.Struct(body); err != nil {
		return spec.PostTripsTripIDLinksJSON400Response(spec.BadRequest{
			Message: "request invalid " + err.Error(),
		})
	}

	if _, err := api.store.GetTrip(r.Context(), tripUUID); err != nil {
		return spec.PostTripsTripIDLinksJSON404Response(spec.NotFoundRequest{
			Message: "trip not found",
		})
	}

	link := pgstore.CreateTripLinkParams{
		Title:  body.Title,
		Url:    body.URL,
		TripID: tripUUID,
	}

	linkId, err := api.store.CreateTripLink(r.Context(), link)
	if err != nil {
		return spec.PostTripsTripIDLinksJSON500Response(spec.InternalServerErrorRequest{
			Message: "unable to create link to trip",
		})
	}

	return spec.PostTripsTripIDLinksJSON201Response(spec.CreateLinkResponse{
		LinkID: linkId.String(),
	})
}

type filterFuncToActivity func(activity pgstore.Activity) bool

func (api *API) filterActivities(activities []pgstore.Activity, f filterFuncToActivity) []pgstore.Activity {
	var activiesFiltered []pgstore.Activity

	for _, activity := range activities {
		if f(activity) {
			activiesFiltered = append(activiesFiltered, activity)
		}
	}
	return activiesFiltered
}

type filterFuncToParticipant func(participant pgstore.Participant) bool

func (api *API) filterParticipants(participants []pgstore.Participant, f filterFuncToParticipant) []pgstore.Participant {
	var participantsFiltered []pgstore.Participant

	for _, participant := range participants {
		if f(participant) {
			participantsFiltered = append(participantsFiltered, participant)
		}
	}

	return participantsFiltered
}

func (api *API) tryParseUUID(nameOfParameterArgument string, id string) (idParsed uuid.UUID, friendlyErrorMessage string, err error) {
	idParsed, err = uuid.Parse(id)
	if err != nil {
		api.logger.Error(err.Error())
		friendlyErrorMessage = nameOfParameterArgument + " is not recognize with a valid uuid"
	}
	return
}

func (api *API) buildRedirectRequestUsingRequestsWithParametersInTheURL(r *http.Request, requestURI string) (*http.Response, error) {

	urlBase := baseurl.MustGet(r)
	fullURL := fmt.Sprintf("%s%s", urlBase, requestURI)
	client := http.Client{}

	newRequest, _ := http.NewRequest(http.MethodPatch, fullURL, nil)
	newRequest.Header = r.Header

	response, err := client.Do(newRequest)

	return response, err
}
