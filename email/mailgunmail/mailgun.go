package mailgunmail

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/haydenwoodhead/burner.kiwi/burner"
	"github.com/haydenwoodhead/burner.kiwi/email"
	log "github.com/sirupsen/logrus"
	mailgun "gopkg.in/mailgun/mailgun-go.v1"
)

var _ burner.EmailProvider = &MailgunMail{}

// MailgunMail is a mailgun implementation of the EmailProvider interface
type MailgunMail struct {
	websiteAddr   string
	mg            mailgun.Mailgun
	db            burner.Database
	isBlacklisted func(string) bool
}

// NewMailgunProvider creates a new Mailgun EmailProvider
func NewMailgunProvider(domain string, key string) *MailgunMail {
	mg := &MailgunMail{
		mg: mailgun.NewMailgun(domain, key, ""),
	}

	go func() {
		for {
			log.Info("mailgun: deleting expired routes")
			err := mg.deleteExpiredRoutes()
			if err != nil {
				log.WithError(err).Error("mailgun: failed to delete expired routes")
			}
			log.Info("mailgun: deleted expired routes")
			time.Sleep(1 * time.Hour)
		}
	}()

	return mg
}

// Start implements EmailProvider Start()
func (m *MailgunMail) Start(websiteAddr string, db burner.Database, r *mux.Router, isBlackisted func(string) bool) error {
	m.db = db
	m.isBlacklisted = isBlackisted
	m.websiteAddr = websiteAddr
	r.HandleFunc("/mg/incoming/{inboxID}/", m.incoming).Methods(http.MethodPost)
	return nil
}

// Stop implements EmailProvider Stop()
func (m *MailgunMail) Stop() error {
	return nil
}

// RegisterRoute implements RegisterRoute()
func (m *MailgunMail) RegisterRoute(i burner.Inbox) (string, error) {
	routeAddr := m.websiteAddr + "/mg/incoming/" + i.ID + "/"
	route, err := m.mg.CreateRoute(mailgun.Route{
		Priority:    1,
		Description: strconv.Itoa(int(i.TTL)),
		Expression:  "match_recipient(\"" + i.Address + "\")",
		Actions:     []string{"forward(\"" + routeAddr + "\")", "store()", "stop()"},
	})
	return route.ID, fmt.Errorf("createRoute: failed to create mailgun route: %w", err)
}

func (m *MailgunMail) deleteExpiredRoutes() error {
	_, rs, err := m.mg.GetRoutes(1000, 0)

	if err != nil {
		return fmt.Errorf("mailgun.DeleteExpiredRoutes: failed to get routes: %w", err)
	}

	for _, r := range rs {
		tInt, err := strconv.ParseInt(r.Description, 10, 64)

		if err != nil {
			log.WithField("routeID", r.ID).Error("mailgun.DeleteExpiredRoutes: failed to parse route description as int")
			continue
		}

		t := time.Unix(tInt, 0)

		// if our route's ttl (expiration time) is before now... then delete it
		if t.Before(time.Now()) {
			err := m.mg.DeleteRoute(r.ID)

			if err != nil {
				log.WithError(err).WithField("routeID", r.ID).Error("mailgun.DeleteExpiredRoutes: failed to delete route")
				continue
			}
		}
	}

	return nil
}

func (m *MailgunMail) incoming(w http.ResponseWriter, r *http.Request) {
	verified, err := m.mg.VerifyWebhookRequest(r)
	if err != nil {
		log.WithError(err).Debug("mailgun.incoming: failed to verify request")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !verified {
		log.Debug("mailgun.incoming: request not verified")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if m.isBlacklisted(r.FormValue("sender")) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	vars := mux.Vars(r)
	id := vars["inboxID"]

	i, err := m.db.GetInboxByID(id)

	if err != nil {
		log.WithError(err).Error("mailgun.incoming: failed to get inbox")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var msg burner.Message

	msg.InboxID = i.ID
	msg.TTL = i.TTL

	mID, err := uuid.NewRandom()
	if err != nil {
		log.WithError(err).Error("mailgun.incoming: failed to generate uuid for inbox")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.ID = mID.String()
	msg.ReceivedAt = time.Now().Unix()
	msg.EmailProviderID = r.FormValue("message-id")
	msg.Sender = r.FormValue("sender")
	msg.From = r.FormValue("from")
	msg.Subject = r.FormValue("subject")
	msg.BodyPlain = r.FormValue("body-plain")

	html := r.FormValue("body-html")

	// Check to see if there is anything in html before we modify it. Otherwise we end up setting a blank html doc
	// on all plaintext emails preventing them from being displayed.
	if html != "" {
		modifiedHTML, err := email.AddTargetBlank(html)
		if err != nil {
			log.WithError(err).Error("mailgun.incoming: failed to AddTargetBlank")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		msg.BodyHTML = modifiedHTML
	}

	err = m.db.SaveNewMessage(msg)
	if err != nil {
		log.WithError(err).Error("mailgun.incoming: failed to save message to db")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = w.Write([]byte(id))
	if err != nil {
		log.WithError(err).Warn("mailgun.incoming: failed to write response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
