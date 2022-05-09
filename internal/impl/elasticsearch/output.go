package elasticsearch

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/olivere/elastic/v7"
	aws "github.com/olivere/elastic/v7/aws/v4"

	"github.com/benthosdev/benthos/v4/internal/batch/policy"
	"github.com/benthosdev/benthos/v4/internal/bloblang/field"
	"github.com/benthosdev/benthos/v4/internal/bundle"
	"github.com/benthosdev/benthos/v4/internal/component"
	"github.com/benthosdev/benthos/v4/internal/component/metrics"
	"github.com/benthosdev/benthos/v4/internal/component/output"
	"github.com/benthosdev/benthos/v4/internal/docs"
	"github.com/benthosdev/benthos/v4/internal/http/docs/auth"
	baws "github.com/benthosdev/benthos/v4/internal/impl/aws"
	sess "github.com/benthosdev/benthos/v4/internal/impl/aws/session"
	"github.com/benthosdev/benthos/v4/internal/interop"
	"github.com/benthosdev/benthos/v4/internal/log"
	"github.com/benthosdev/benthos/v4/internal/message"
	ooutput "github.com/benthosdev/benthos/v4/internal/old/output"
	"github.com/benthosdev/benthos/v4/internal/old/util/retries"
	itls "github.com/benthosdev/benthos/v4/internal/tls"
)

func init() {
	err := bundle.AllOutputs.Add(bundle.OutputConstructorFromSimple(func(conf ooutput.Config, mgr bundle.NewManagement) (output.Streamed, error) {
		return NewElasticsearch(conf, mgr, mgr.Logger(), mgr.Metrics())
	}), docs.ComponentSpec{
		Name: "elasticsearch",
		Summary: `
Publishes messages into an Elasticsearch index. If the index does not exist then
it is created with a dynamic mapping.`,
		Description: output.Description(true, true, `
Both the `+"`id` and `index`"+` fields can be dynamically set using function
interpolations described [here](/docs/configuration/interpolation#bloblang-queries). When
sending batched messages these interpolations are performed per message part.

### AWS

It's possible to enable AWS connectivity with this output using the `+"`aws`"+`
fields. However, you may need to set `+"`sniff` and `healthcheck`"+` to
false for connections to succeed.`),
		Config: docs.FieldComponent().WithChildren(
			docs.FieldString("urls", "A list of URLs to connect to. If an item of the list contains commas it will be expanded into multiple URLs.", []string{"http://localhost:9200"}).Array(),
			docs.FieldString("index", "The index to place messages.").IsInterpolated(),
			docs.FieldString("action", "The action to take on the document.").IsInterpolated().HasOptions("index", "update", "delete").Advanced(),
			docs.FieldString("pipeline", "An optional pipeline id to preprocess incoming documents.").IsInterpolated().Advanced(),
			docs.FieldString("id", "The ID for indexed messages. Interpolation should be used in order to create a unique ID for each message.").IsInterpolated(),
			docs.FieldString("type", "The document type.").Deprecated(),
			docs.FieldString("routing", "The routing key to use for the document.").IsInterpolated().Advanced(),
			docs.FieldBool("sniff", "Prompts Benthos to sniff for brokers to connect to when establishing a connection.").Advanced(),
			docs.FieldBool("healthcheck", "Whether to enable healthchecks.").Advanced(),
			docs.FieldString("timeout", "The maximum time to wait before abandoning a request (and trying again).").Advanced(),
			itls.FieldSpec(),
			docs.FieldInt("max_in_flight", "The maximum number of messages to have in flight at a given time. Increase this to improve throughput."),
		).WithChildren(retries.FieldSpecs()...).WithChildren(
			auth.BasicAuthFieldSpec(),
			policy.FieldSpec(),
			docs.FieldObject("aws", "Enables and customises connectivity to Amazon Elastic Service.").WithChildren(
				docs.FieldSpecs{
					docs.FieldBool("enabled", "Whether to connect to Amazon Elastic Service."),
				}.Merge(sess.FieldSpecs())...,
			).Advanced(),
			docs.FieldBool("gzip_compression", "Enable gzip compression on the request side.").Advanced(),
		).ChildDefaultAndTypesFromStruct(ooutput.NewElasticsearchConfig()),
		Categories: []string{
			"Services",
		},
	})
	if err != nil {
		panic(err)
	}
}

// NewElasticsearch creates a new Elasticsearch output type.
func NewElasticsearch(conf ooutput.Config, mgr interop.Manager, log log.Modular, stats metrics.Type) (output.Streamed, error) {
	elasticWriter, err := NewElasticsearchV2(conf.Elasticsearch, mgr, log, stats)
	if err != nil {
		return nil, err
	}
	w, err := ooutput.NewAsyncWriter(
		"elasticsearch", conf.Elasticsearch.MaxInFlight, elasticWriter, log, stats,
	)
	if err != nil {
		return w, err
	}
	return ooutput.NewBatcherFromConfig(conf.Elasticsearch.Batching, w, mgr, log, stats)
}

// Elasticsearch is a writer type that writes messages into elasticsearch.
type Elasticsearch struct {
	log   log.Modular
	stats metrics.Type

	urls        []string
	sniff       bool
	healthcheck bool
	conf        ooutput.ElasticsearchConfig

	backoffCtor func() backoff.BackOff
	timeout     time.Duration
	tlsConf     *tls.Config

	actionStr   *field.Expression
	idStr       *field.Expression
	indexStr    *field.Expression
	pipelineStr *field.Expression
	routingStr  *field.Expression

	client *elastic.Client
}

// NewElasticsearchV2 creates a new Elasticsearch writer type.
func NewElasticsearchV2(conf ooutput.ElasticsearchConfig, mgr interop.Manager, log log.Modular, stats metrics.Type) (*Elasticsearch, error) {
	e := Elasticsearch{
		log:         log,
		stats:       stats,
		conf:        conf,
		sniff:       conf.Sniff,
		healthcheck: conf.Healthcheck,
	}

	var err error
	if e.actionStr, err = mgr.BloblEnvironment().NewField(conf.Action); err != nil {
		return nil, fmt.Errorf("failed to parse action expression: %v", err)
	}
	if e.idStr, err = mgr.BloblEnvironment().NewField(conf.ID); err != nil {
		return nil, fmt.Errorf("failed to parse id expression: %v", err)
	}
	if e.indexStr, err = mgr.BloblEnvironment().NewField(conf.Index); err != nil {
		return nil, fmt.Errorf("failed to parse index expression: %v", err)
	}
	if e.pipelineStr, err = mgr.BloblEnvironment().NewField(conf.Pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline expression: %v", err)
	}
	if e.routingStr, err = mgr.BloblEnvironment().NewField(conf.Routing); err != nil {
		return nil, fmt.Errorf("failed to parse routing key expression: %v", err)
	}

	for _, u := range conf.URLs {
		for _, splitURL := range strings.Split(u, ",") {
			if len(splitURL) > 0 {
				e.urls = append(e.urls, splitURL)
			}
		}
	}

	if tout := conf.Timeout; len(tout) > 0 {
		var err error
		if e.timeout, err = time.ParseDuration(tout); err != nil {
			return nil, fmt.Errorf("failed to parse timeout string: %v", err)
		}
	}

	if e.backoffCtor, err = conf.Config.GetCtor(); err != nil {
		return nil, err
	}

	if conf.TLS.Enabled {
		var err error
		if e.tlsConf, err = conf.TLS.Get(); err != nil {
			return nil, err
		}
	}
	return &e, nil
}

//------------------------------------------------------------------------------

// ConnectWithContext attempts to establish a connection to a Elasticsearch
// broker.
func (e *Elasticsearch) ConnectWithContext(ctx context.Context) error {
	return e.Connect()
}

// Connect attempts to establish a connection to a Elasticsearch broker.
func (e *Elasticsearch) Connect() error {
	if e.client != nil {
		return nil
	}

	opts := []elastic.ClientOptionFunc{
		elastic.SetURL(e.urls...),
		elastic.SetSniff(e.sniff),
		elastic.SetHealthcheck(e.healthcheck),
	}

	if e.conf.Auth.Enabled {
		opts = append(opts, elastic.SetBasicAuth(
			e.conf.Auth.Username, e.conf.Auth.Password,
		))
	}

	if e.conf.TLS.Enabled {
		opts = append(opts, elastic.SetHttpClient(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.tlsConf,
			},
			Timeout: e.timeout,
		}))

	} else {
		opts = append(opts, elastic.SetHttpClient(&http.Client{
			Timeout: e.timeout,
		}))
	}

	if e.conf.AWS.Enabled {
		tsess, err := baws.GetSessionFromConf(e.conf.AWS.Config)
		if err != nil {
			return err
		}
		signingClient := aws.NewV4SigningClient(tsess.Config.Credentials, e.conf.AWS.Region)
		opts = append(opts, elastic.SetHttpClient(signingClient))
	}

	if e.conf.GzipCompression {
		opts = append(opts, elastic.SetGzip(true))
	}

	client, err := elastic.NewClient(opts...)
	if err != nil {
		return err
	}

	e.client = client
	e.log.Infof("Sending messages to Elasticsearch index at urls: %s\n", e.urls)
	return nil
}

func shouldRetry(s int) bool {
	if s >= 500 && s <= 599 {
		return true
	}
	return false
}

type pendingBulkIndex struct {
	Action   string
	Index    string
	Pipeline string
	Routing  string
	Type     string
	Doc      interface{}
	ID       string
}

// WriteWithContext will attempt to write a message to Elasticsearch, wait for
// acknowledgement, and returns an error if applicable.
func (e *Elasticsearch) WriteWithContext(ctx context.Context, msg *message.Batch) error {
	return e.Write(msg)
}

// Write will attempt to write a message to Elasticsearch, wait for
// acknowledgement, and returns an error if applicable.
func (e *Elasticsearch) Write(msg *message.Batch) error {
	if e.client == nil {
		return component.ErrNotConnected
	}

	boff := e.backoffCtor()

	requests := make([]*pendingBulkIndex, msg.Len())
	if err := msg.Iter(func(i int, part *message.Part) error {
		jObj, ierr := part.JSON()
		if ierr != nil {
			e.log.Errorf("Failed to marshal message into JSON document: %v\n", ierr)
			return fmt.Errorf("failed to marshal message into JSON document: %w", ierr)
		}
		requests[i] = &pendingBulkIndex{
			Action:   e.actionStr.String(i, msg),
			Index:    e.indexStr.String(i, msg),
			Pipeline: e.pipelineStr.String(i, msg),
			Routing:  e.routingStr.String(i, msg),
			Type:     e.conf.Type,
			Doc:      jObj,
			ID:       e.idStr.String(i, msg),
		}
		return nil
	}); err != nil {
		return err
	}

	b := e.client.Bulk()
	for _, v := range requests {
		bulkReq, err := e.buildBulkableRequest(v)
		if err != nil {
			return err
		}
		b.Add(bulkReq)
	}

	lastErrReason := "no reason given"
	for b.NumberOfActions() != 0 {
		result, err := b.Do(context.Background())
		if err != nil {
			return err
		}
		if !result.Errors {
			return nil
		}

		var newRequests []*pendingBulkIndex
		for i, resp := range result.Items {
			for _, item := range resp {
				if item.Status >= 200 && item.Status <= 299 {
					continue
				}

				reason := "no reason given"
				if item.Error != nil {
					reason = item.Error.Reason
					lastErrReason = fmt.Sprintf("status [%v]: %v", item.Status, reason)
				}

				e.log.Errorf("Elasticsearch message '%v' rejected with status [%v]: %v\n", item.Id, item.Status, reason)
				if !shouldRetry(item.Status) {
					return fmt.Errorf("failed to send message '%v': %v", item.Id, reason)
				}

				// IMPORTANT: i exactly matches the index of our source requests
				// and when we re-run our bulk request with errored requests
				// that must remain true.
				sourceReq := requests[i]
				bulkReq, err := e.buildBulkableRequest(sourceReq)
				if err != nil {
					return err
				}
				b.Add(bulkReq)
				newRequests = append(newRequests, sourceReq)
			}
		}
		requests = newRequests

		wait := boff.NextBackOff()
		if wait == backoff.Stop {
			return fmt.Errorf("retries exhausted for messages, aborting with last error reported as: %v", lastErrReason)
		}
		time.Sleep(wait)
	}

	return nil
}

// CloseAsync shuts down the Elasticsearch writer and stops processing messages.
func (e *Elasticsearch) CloseAsync() {
}

// WaitForClose blocks until the Elasticsearch writer has closed down.
func (e *Elasticsearch) WaitForClose(timeout time.Duration) error {
	return nil
}

// Build a bulkable request for a given pending bulk index item.
func (e *Elasticsearch) buildBulkableRequest(p *pendingBulkIndex) (elastic.BulkableRequest, error) {
	switch p.Action {
	case "update":
		r := elastic.NewBulkUpdateRequest().
			Index(p.Index).
			Routing(p.Routing).
			Id(p.ID).
			Doc(p.Doc)
		if p.Type != "" {
			r = r.Type(p.Type)
		}
		return r, nil
	case "delete":
		r := elastic.NewBulkDeleteRequest().
			Index(p.Index).
			Routing(p.Routing).
			Id(p.ID)
		if p.Type != "" {
			r = r.Type(p.Type)
		}
		return r, nil
	case "index":
		r := elastic.NewBulkIndexRequest().
			Index(p.Index).
			Pipeline(p.Pipeline).
			Routing(p.Routing).
			Id(p.ID).
			Doc(p.Doc)
		if p.Type != "" {
			r = r.Type(p.Type)
		}
		return r, nil
	default:
		return nil, fmt.Errorf("elasticsearch action '%s' is not allowed", p.Action)
	}
}