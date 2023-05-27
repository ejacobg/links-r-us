package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ejacobg/links-r-us/bleve"
	"github.com/ejacobg/links-r-us/cdb"
	"github.com/ejacobg/links-r-us/crawler"
	"github.com/ejacobg/links-r-us/elasticsearch"
	"github.com/ejacobg/links-r-us/frontend"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/ejacobg/links-r-us/index"
	"github.com/ejacobg/links-r-us/inmem"
	"github.com/ejacobg/links-r-us/pagerank"
	"github.com/ejacobg/links-r-us/partition"
	"github.com/ejacobg/links-r-us/service"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var (
	appName = "linksrus-monolith"
	appSha  = "populated-at-link-time"
)

func main() {
	host, _ := os.Hostname()
	rootLogger := logrus.New()
	logger := rootLogger.WithFields(logrus.Fields{
		"app":  appName,
		"sha":  appSha,
		"host": host,
	})

	if err := runMain(logger); err != nil {
		logrus.WithField("err", err).Error("shutting down due to error")
		return
	}
	logger.Info("shutdown complete")
}

func runMain(logger *logrus.Entry) error {
	svcGroup, err := setupServices(logger)
	if err != nil {
		return err
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGHUP)
		select {
		case s := <-sigCh:
			logger.WithField("signal", s.String()).Infof("shutting down due to signal")
			cancelFn()
		case <-ctx.Done():
		}
	}()

	return svcGroup.Run(ctx)
}

func setupServices(logger *logrus.Entry) (service.Group, error) {
	var (
		frontendCfg frontend.Config
		crawlerCfg  crawler.ServiceConfig
		pageRankCfg pagerank.ServiceConfig
	)

	flag.StringVar(&frontendCfg.ListenAddr, "frontend-listen-addr", ":8080", "The address to listen for incoming front-end requests")
	flag.IntVar(&frontendCfg.ResultsPerPage, "frontend-results-per-page", 10, "The number of entries for each search result page")
	flag.IntVar(&frontendCfg.MaxSummaryLength, "frontend-max-summary-length", 256, "The maximum length of the summary for each matched document in characters")

	flag.IntVar(&crawlerCfg.FetchWorkers, "crawler-num-workers", runtime.NumCPU(), "The number of workers to use for crawling web-pages (defaults to number of CPUs)")
	flag.DurationVar(&crawlerCfg.UpdateInterval, "crawler-update-interval", 5*time.Minute, "The time between subsequent crawler runs")
	flag.DurationVar(&crawlerCfg.ReIndexThreshold, "crawler-reindex-threshold", 7*24*time.Hour, "The minimum amount of time before re-indexing an already-crawled link")

	flag.IntVar(&pageRankCfg.ComputeWorkers, "pagerank-num-workers", runtime.NumCPU(), "The number of workers to use for calculating PageRank scores (defaults to number of CPUs)")
	flag.DurationVar(&pageRankCfg.UpdateInterval, "pagerank-update-interval", time.Hour, "The time between subsequent PageRank score updates")

	linkGraphURI := flag.String("link-graph-uri", "in-memory://", "The URI for connecting to the link-graph (supported URIs: in-memory://, postgresql://user@host:26257/linkgraph?sslmode=disable)")
	textIndexerURI := flag.String("text-indexer-uri", "in-memory://", "The URI for connecting to the text indexer (supported URIs: in-memory://, es://node1:9200,...,nodeN:9200)")

	partitionDetMode := flag.String("partition-detection-mode", "single", "The partition detection mode to use. Supported values are 'dns=HEADLESS_SERVICE_NAME' (k8s) and 'single' (local dev mode)")
	flag.Parse()

	// Retrieve a suitable link graph and text indexer implementation and
	// plug it into the service configurations.
	linkGraph, err := getLinkGraph(*linkGraphURI, logger)
	if err != nil {
		return nil, err
	}
	textIndexer, err := getTextIndexer(*textIndexerURI, logger)
	if err != nil {
		return nil, err
	}

	// Create a helper for detecting the partition assigned to this instance.
	partDet, err := getPartitionDetector(*partitionDetMode)
	if err != nil {
		return nil, err
	}

	var svc service.Service
	var svcGroup service.Group

	frontendCfg.GraphAPI = linkGraph
	frontendCfg.IndexAPI = textIndexer
	frontendCfg.Logger = logger.WithField("service", "front-end")
	if svc, err = frontend.NewService(frontendCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	crawlerCfg.GraphAPI = linkGraph
	crawlerCfg.IndexAPI = textIndexer
	crawlerCfg.PartitionDetector = partDet
	crawlerCfg.Logger = logger.WithField("service", "crawler")
	if svc, err = crawler.NewService(crawlerCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	pageRankCfg.GraphAPI = linkGraph
	pageRankCfg.IndexAPI = textIndexer
	pageRankCfg.PartitionDetector = partDet
	pageRankCfg.Logger = logger.WithField("service", "pagerank-calculator")
	if svc, err = pagerank.NewService(pageRankCfg); err == nil {
		svcGroup = append(svcGroup, svc)
	} else {
		return nil, err
	}

	return svcGroup, nil
}

type linkGraph interface {
	UpsertLink(link *graph.Link) error
	UpsertEdge(edge *graph.Edge) error
	RemoveStaleEdges(fromID uuid.UUID, updatedBefore time.Time) error
	Links(fromID, toID uuid.UUID, retrievedBefore time.Time) (graph.LinkIterator, error)
	Edges(fromID, toID uuid.UUID, updatedBefore time.Time) (graph.EdgeIterator, error)
}

func getLinkGraph(linkGraphURI string, logger *logrus.Entry) (linkGraph, error) {
	if linkGraphURI == "" {
		return nil, fmt.Errorf("link graph URI must be specified with --link-graph-uri")
	}

	uri, err := url.Parse(linkGraphURI)
	if err != nil {
		return nil, fmt.Errorf("could not parse link graph URI: %w", err)
	}

	switch uri.Scheme {
	case "in-memory":
		logger.Info("using in-memory graph")
		return inmem.NewGraph(), nil
	case "postgresql":
		logger.Info("using CDB graph")
		return cdb.NewGraph(linkGraphURI)
	default:
		return nil, fmt.Errorf("unsupported link graph URI scheme: %q", uri.Scheme)
	}
}

type textIndexer interface {
	Index(text *index.Document) error
	UpdateScore(linkID uuid.UUID, score float64) error
	Search(query index.Query) (index.Iterator, error)
}

func getTextIndexer(textIndexerURI string, logger *logrus.Entry) (textIndexer, error) {
	if textIndexerURI == "" {
		return nil, fmt.Errorf("text indexer URI must be specified with --text-indexer-uri")
	}

	uri, err := url.Parse(textIndexerURI)
	if err != nil {
		return nil, fmt.Errorf("could not parse text indexer URI: %w", err)
	}

	switch uri.Scheme {
	case "in-memory":
		logger.Info("using in-memory indexer")
		return bleve.NewIndexer()
	case "es":
		nodes := strings.Split(uri.Host, ",")
		for i := 0; i < len(nodes); i++ {
			nodes[i] = "http://" + nodes[i]
		}
		logger.Info("using ES indexer")
		return elasticsearch.NewIndexer(nodes, false)
	default:
		return nil, fmt.Errorf("unsupported link graph URI scheme: %q", uri.Scheme)
	}
}

func getPartitionDetector(mode string) (partition.Detector, error) {
	switch {
	case mode == "single":
		return partition.Fixed{Partition: 0, NumPartitions: 1}, nil
	case strings.HasPrefix(mode, "dns="):
		tokens := strings.Split(mode, "=")
		return partition.DetectFromSRVRecords(tokens[1]), nil
	default:
		return nil, fmt.Errorf("unsupported partition detection mode: %q", mode)
	}
}
