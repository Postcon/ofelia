package middlewares

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Postcon/ofelia/core"
)

var (
	slackUsername   = "Ofelia"
	slackAvatarURL  = ""
	slackPayloadVar = "payload"
	slackLogsUrl    = "http://graylog.postcon.intern:9000/search?rangetype=relative&fields=source%2Cmessage&width=1920&relative=86400&from=&to=&q=service%3A###service_name####?fields=source%2Cmessage%2Cservice"
)

// SlackConfig configuration for the Slack middleware
type SlackConfig struct {
	SlackWebhook     string `gcfg:"slack-webhook"`
	SlackOnlyOnError bool   `gcfg:"slack-only-on-error"`
}

// NewSlack returns a Slack middleware if the given configuration is not empty
func NewSlack(c *SlackConfig) core.Middleware {
	var m core.Middleware
	if !IsEmpty(c) {
		m = &Slack{*c}
	}

	return m
}

// Slack middleware calls to a Slack input-hook after every execution of a job
type Slack struct {
	SlackConfig
}

// ContinueOnStop return allways true, we want alloways report the final status
func (m *Slack) ContinueOnStop() bool {
	return true
}

// Run sends a message to the slack channel, its close stop the exection to
// collect the metrics
func (m *Slack) Run(ctx *core.Context) error {
	err := ctx.Next()
	ctx.Stop(err)

	if ctx.Execution.Failed || !m.SlackOnlyOnError {
		m.pushMessage(ctx)
	}

	return err
}

func (m *Slack) pushMessage(ctx *core.Context) {
	values := make(url.Values, 0)
	content, _ := json.Marshal(m.buildMessage(ctx))
	values.Add(slackPayloadVar, string(content))

	r, err := http.PostForm(m.SlackWebhook, values)
	if err != nil {
		ctx.Logger.Errorf("Slack error calling %q error: %q", m.SlackWebhook, err)
	} else if r.StatusCode != 200 {
		ctx.Logger.Errorf("Slack error non-200 status code calling %q", m.SlackWebhook)
	}
}

func (m *Slack) buildMessage(ctx *core.Context) *slackMessage {
	msg := &slackMessage{
		Username: slackUsername,
		IconURL:  slackAvatarURL,
	}

	msg.Text = fmt.Sprintf(
		"Job *%s* finished in *%s*, command _%q_",
		ctx.Job.GetName(), ctx.Execution.Duration, ctx.Job.GetCommand(),
	)

	if ctx.Execution.Failed {
		logsUrl := fmt.Sprintf(
			"\n<%s|show logs>",
			strings.Replace(slackLogsUrl, "###service_name###", strings.SplitN(ctx.Job.GetName(), "_", 2)[1], 1),
		)

		msg.Attachments = append(msg.Attachments, slackAttachment{
			Title: "Execution failed",
			Text:  fmt.Sprintf("%s%s", ctx.Execution.Error.Error(), logsUrl),
			Color: "#F35A00",
		})
	} else if ctx.Execution.Skipped {
		msg.Attachments = append(msg.Attachments, slackAttachment{
			Title: "Execution skipped",
			Color: "#FFA500",
		})
	} else {
		msg.Attachments = append(msg.Attachments, slackAttachment{
			Title: "Execution successful",
			Color: "#7CD197",
		})
	}

	return msg
}

type slackMessage struct {
	Text        string            `json:"text"`
	Username    string            `json:"username"`
	Attachments []slackAttachment `json:"attachments"`
	IconURL     string            `json:"icon_url"`
}

type slackAttachment struct {
	Color string `json:"color,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text"`
}
