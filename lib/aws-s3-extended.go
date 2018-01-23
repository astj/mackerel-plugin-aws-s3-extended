package mpawss3extended

import (
	"errors"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	mp "github.com/mackerelio/go-mackerel-plugin"
)

const (
	namespace          = "AWS/S3"
	metricsTypeAverage = "Average"
	metricsTypeSum     = "Sum"
	metricsTypeMaximum = "Maximum"
	metricsTypeMinimum = "Minimum"
)

// has 1 CloudWatch MetricName and corresponding N Mackerel Metrics
type metricsGroup struct {
	CloudWatchName string
	Metrics        []metric
}

type metric struct {
	MackerelName string
	Type         string
}

// S3ExtendedPlugin is mackerel plugin for aws s3 with metric configuration
type S3ExtendedPlugin struct {
	BucketName string
	FilterID   string
	Prefix     string

	AccessKeyID     string
	SecretAccessKey string
	Region          string

	CloudWatch *cloudwatch.CloudWatch
}

// MetricKeyPrefix interface for PluginWithPrefix
func (p S3ExtendedPlugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		return "s3-extented"
	}
	return p.Prefix
}

// prepare creates CloudWatch instance
func (p *S3ExtendedPlugin) prepare() error {
	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	config := aws.NewConfig()
	if p.AccessKeyID != "" && p.SecretAccessKey != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(p.AccessKeyID, p.SecretAccessKey, ""))
	}
	if p.Region != "" {
		config = config.WithRegion(p.Region)
	}

	p.CloudWatch = cloudwatch.New(sess, config)

	return nil
}

// getLastPoint fetches a CloudWatch metric and parse
func getLastPointFromCloudWatch(cw cloudwatchiface.CloudWatchAPI, bucketName string, filterID string, metric metricsGroup) (*cloudwatch.Datapoint, error) {
	now := time.Now()
	statsInput := make([]*string, len(metric.Metrics))
	for i, typ := range metric.Metrics {
		statsInput[i] = aws.String(typ.Type)
	}
	input := &cloudwatch.GetMetricStatisticsInput{
		StartTime:  aws.Time(now.Add(time.Duration(180) * time.Second * -1)), // 3 min
		EndTime:    aws.Time(now),
		MetricName: aws.String(metric.CloudWatchName),
		Period:     aws.Int64(600),
		Statistics: statsInput,
		Namespace:  aws.String(namespace),
	}
	input.Dimensions = []*cloudwatch.Dimension{
		{
			Name:  aws.String("BucketName"),
			Value: aws.String(bucketName),
		},
		{
			Name:  aws.String("FilterId"),
			Value: aws.String(filterID),
		},
	}
	response, err := cw.GetMetricStatistics(input)
	if err != nil {
		return nil, err
	}

	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return nil, errors.New("fetched no datapoints")
	}

	latest := new(time.Time)
	var latestDp *cloudwatch.Datapoint
	for _, dp := range datapoints {
		if dp.Timestamp.Before(*latest) {
			continue
		}

		latest = dp.Timestamp
		latestDp = dp
	}

	return latestDp, nil
}

// TransformMetrics converts some of datapoints to post differences of two metrics
func transformMetrics(stats map[string]float64) map[string]float64 {
	if totalCount, ok := stats["TEMPORARY_invocations_total"]; ok {
		if errorCount, ok := stats["invocations_error"]; ok {
			stats["invocations_success"] = totalCount - errorCount
		} else {
			stats["invocations_success"] = totalCount
		}
		delete(stats, "TEMPORARY_invocations_total")
	}
	return stats
}

func mergeStatsFromDatapoint(stats map[string]float64, dp *cloudwatch.Datapoint, mg metricsGroup) map[string]float64 {
	for _, met := range mg.Metrics {
		switch met.Type {
		case metricsTypeAverage:
			stats[met.MackerelName] = *dp.Average
		case metricsTypeSum:
			stats[met.MackerelName] = *dp.Sum
		case metricsTypeMaximum:
			stats[met.MackerelName] = *dp.Maximum
		case metricsTypeMinimum:
			stats[met.MackerelName] = *dp.Minimum
		}
	}
	return stats
}

var s3RequestMetricsGroup = []metricsGroup{
	{CloudWatchName: "GetRequests", Metrics: []metric{
		{MackerelName: "GetRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "PutRequests", Metrics: []metric{
		{MackerelName: "PutRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "DeleteRequests", Metrics: []metric{
		{MackerelName: "DeleteRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "HeadRequests", Metrics: []metric{
		{MackerelName: "HeadRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "PostRequests", Metrics: []metric{
		{MackerelName: "PostRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "ListRequests", Metrics: []metric{
		{MackerelName: "ListRequests", Type: metricsTypeSum},
	}},
	{CloudWatchName: "4xxErrors", Metrics: []metric{
		{MackerelName: "4xxErrors", Type: metricsTypeSum},
	}},
	{CloudWatchName: "5xxErrors", Metrics: []metric{
		{MackerelName: "5xxErrors", Type: metricsTypeSum},
	}},
	{CloudWatchName: "BytesDownloaded", Metrics: []metric{
		{MackerelName: "BytesDownloaded", Type: metricsTypeSum},
	}},
	{CloudWatchName: "BytesUploaded", Metrics: []metric{
		{MackerelName: "BytesUploaded", Type: metricsTypeSum},
	}},
	{CloudWatchName: "TotalRequestLatency", Metrics: []metric{
		{MackerelName: "TotalRequestLatencyAvg", Type: metricsTypeAverage},
		{MackerelName: "TotalRequestLatencyMax", Type: metricsTypeMaximum},
		{MackerelName: "TotalRequestLatencyMin", Type: metricsTypeMinimum},
	}},
}

// FetchMetrics fetch the metrics
func (p S3ExtendedPlugin) FetchMetrics() (map[string]float64, error) {
	stats := make(map[string]float64)

	for _, met := range s3RequestMetricsGroup {
		v, err := getLastPointFromCloudWatch(p.CloudWatch, p.BucketName, p.FilterID, met)
		if err == nil {
			stats = mergeStatsFromDatapoint(stats, v, met)
		} else {
			log.Printf("%s: %s", met, err)
		}
	}
	return transformMetrics(stats), nil
}

// GraphDefinition of S3ExtendedPlugin
func (p S3ExtendedPlugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.Prefix)

	graphdef := map[string]mp.Graphs{
		"requests": {
			Label: (labelPrefix + " Requests"),
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "GetRequests", Label: "Get", Stacked: true},
				{Name: "PutRequests", Label: "Put", Stacked: true},
				{Name: "DeleteRequests", Label: "Delete", Stacked: true},
				{Name: "HeadRequests", Label: "Head", Stacked: true},
				{Name: "PostRequests", Label: "Post", Stacked: true},
				{Name: "ListRequests", Label: "List", Stacked: true},
			},
		},
		"errors": {
			Label: (labelPrefix + " Errors"),
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "4xxErrors", Label: "4xx"},
				{Name: "5xxErrors", Label: "5xx"},
			},
		},
		"bytes": {
			Label: (labelPrefix + " Bytes"),
			Unit:  "bytes",
			Metrics: []mp.Metrics{
				{Name: "BytesDownloaded", Label: "Downloaded"},
				{Name: "BytesUploaded", Label: "Uploaded"},
			},
		},
		"latency": {
			Label: (labelPrefix + " TotalRequestLatency"),
			Unit:  "float",
			Metrics: []mp.Metrics{
				{Name: "TotalRequestLatencyAvg", Label: "Average"},
				{Name: "TotalRequestLatencyMax", Label: "Maximum"},
				{Name: "TotalRequestLatencyMin", Label: "Minimum"},
			},
		},
	}
	return graphdef
}

// Do the plugin
func Do() {
	optAccessKeyID := flag.String("access-key-id", "", "AWS Access Key ID")
	optSecretAccessKey := flag.String("secret-access-key", "", "AWS Secret Access Key")
	optRegion := flag.String("region", "", "AWS Region")
	optBucketName := flag.String("bucket-name", "", "S3 bucket Name")
	optFilterID := flag.String("filter-id", "", "S3 FilterId in metrics configuration")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	optPrefix := flag.String("metric-key-prefix", "s3-extended", "Metric key prefix")
	flag.Parse()

	var plugin S3ExtendedPlugin

	plugin.AccessKeyID = *optAccessKeyID
	plugin.SecretAccessKey = *optSecretAccessKey
	plugin.Region = *optRegion

	plugin.BucketName = *optBucketName
	plugin.FilterID = *optFilterID
	plugin.Prefix = *optPrefix

	err := plugin.prepare()
	if err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(plugin)
	helper.Tempfile = *optTempfile

	helper.Run()
}
