package main

import (
	"flag"
	"os"
	"strconv"
	"time"

	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TaintOperation string

const (
   PreferNoSchedule    TaintOperation = "PreferNoSchedule"
   NoSchedule          TaintOperation = "NoSchedule"
   NoExecute           TaintOperation = "NoExecute"
)

const k8sReservedLabelPrefix = "kubernetes.io/"

func usage() {
	fmt.Fprintf(os.Stderr, "usage: labelgun -stderrthreshold=[INFO|WARN|FATAL]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func init() {
	flag.Usage = usage
	// NOTE: This next line is key you have to call flag.Parse() for the command line
	// options or "flags" that are defined in the glog module to be picked up.
	flag.Parse()
}

func interval() int64 {
	val, _ := strconv.ParseInt(os.Getenv("LABELGUN_INTERVAL"), 10, 64)
	if val == 0 {
		return 60
	}
	return val
}

func noScheduleTaintTagPrefix() string {
	val := os.Getenv("LABELGUN_NO_SCHEDULE_TAG_PREFIX")
	return val
}

func preferNoScheduleTaintTagPrefix() string {
	val := os.Getenv("LABELGUN_PREFER_NO_SCHEDULE_TAG_PREFIX")
	return val
}

func noExecuteTaintTagPrefix() string {
	val := os.Getenv("LABELGUN_NO_EXECUTE_TAG_PREFIX")
	return val
}

func labelTagPrefix() string {
	val := os.Getenv("LABELGUN_LABEL_TAG_PREFIX")
	return val
}

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}

	noScheduleTaintTagPrefix := noScheduleTaintTagPrefix()
	preferNoScheduleTaintTagPrefix := preferNoScheduleTaintTagPrefix()
	noExecuteTaintTagPrefix := noExecuteTaintTagPrefix()

	for {
		// Get Kube Nodes
		clientset := kubeClient(config)
		nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Get EC2 metadata
		metadata := ec2metadata.New(session.New())

		region, err := metadata.Region()
		if err != nil {
			log.Fatalf("Unable to retrieve the region from the EC2 instance %v\n", err)
		}

		creds := credentials.NewChainCredentials(
			[]credentials.Provider{
				&credentials.EnvProvider{},
				&credentials.SharedCredentialsProvider{},
				&ec2rolecreds.EC2RoleProvider{Client: metadata},
			})

		awsConfig := aws.NewConfig()
		awsConfig.WithCredentials(creds)
		awsConfig.WithRegion(region)
		sess, err := session.NewSession(awsConfig)
		if err != nil {
			log.Fatal(err)
		}

		svc := ec2.New(sess)

		for _, node := range nodes.Items {

			// Here we create an input that will filter any instances that aren't either
			// of these two states. This is generally what we want
			params := &ec2.DescribeInstancesInput{
				Filters: []*ec2.Filter{
					&ec2.Filter{
						Name: aws.String("private-dns-name"),
						Values: []*string{
							&node.Name,
						},
					},
				},
			}

			resp, _ := svc.DescribeInstances(params)
			if err != nil {
				log.Fatal(err)
			}
			if len(resp.Reservations) < 1 || len(resp.Reservations[0].Instances) < 1 {
				// Might due to "Request body type has been overwritten. May cause race conditions"
				next(interval())
				break
			}

			// Apply EC2 Tags
			nodeName := node.Name
			inst := resp.Reservations[0].Instances[0]

			for _, keys := range inst.Tags {
				tagKey := tagToLabel(*keys.Key)
				tagValue := tagToLabel(*keys.Value)

				if tagKey == "" || tagValue == "" {
					continue
				}

				if strings.HasPrefix(tagKey, k8sReservedLabelPrefix) {
					continue
				}

				labelTagPrefix = labelTagPrefix()

				if labelTagPrefix == "*" || strings.HasPrefix(tagKey, labelTagPrefix) {
					label(nodeName, tagKey, tagValue)
				}

				// order matters here
				// we apply the most conservative taint last to ensure prefixes that match multiple labels get the most conservative taint operation applied
				if preferNoScheduleTaintTagPrefix != "" &&
					(preferNoScheduleTaintTagPrefix == "*" || strings.HasPrefix(tagKey, preferNoScheduleTaintTagPrefix)) {
					taint(nodeName, tagKey, tagValue, PreferNoSchedule)
				}
				if noScheduleTaintTagPrefix != "" &&
					(noScheduleTaintTagPrefix == "*" || strings.HasPrefix(tagKey, noScheduleTaintTagPrefix)) {
					taint(nodeName, tagKey, tagValue, NoSchedule)
				}
				if noExecuteTaintTagPrefix != "" &&
					(noExecuteTaintTagPrefix == "*" || strings.HasPrefix(tagKey, noExecuteTaintTagPrefix)) {
					taint(nodeName, tagKey, tagValue, NoExecute)
				}
			}
		}
		// Sleep until interval
		next(interval())
	}
}

func tagToLabel(item string) string {
	parsed, err := strconv.Unquote(string(awsutil.Prettify(item)))
	if err != nil {
		log.Error(err)
		return ""
	}
	parsed = strings.Replace(parsed, ":", ".", -1)
	if len(parsed) > 63 {
		return ""
	}
	return parsed
}

func next(interval int64) {
	log.Infof("Sleeping for %d seconds", interval)
	time.Sleep(time.Duration(interval) * time.Second)
}

func label(nodeName string, tagKey string, tagValue string) {
	log.Infoln(fmt.Sprintf("kubectl node %s %s=%s", nodeName, tagKey, tagValue))

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}

	clientset := kubeClient(config)
	node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf(err.Error())
	}

	labels := node.GetLabels()
	labels[tagKey] = tagValue

	_, err = clientset.CoreV1().Nodes().Update(node)
	if err != nil {
		log.Error(err)
	}
}

func taint(nodeName string, tagKey string, tagValue string, taintOperation TaintOperation) {
	log.Infoln(fmt.Sprintf("kubectl node %s %s=%s %s", nodeName, tagKey, tagValue, taintOperation))

	var taint v1.Taint
	effect := v1.TaintEffect(taintOperation)

	taint.Key = tagKey
    taint.Value = tagValue
    taint.Effect = effect

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}

	clientset := kubeClient(config)
	node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf(err.Error())
	}

	newNode := node.DeepCopy()
	nodeTaints := newNode.Spec.Taints

	var newTaints []v1.Taint
	updated := false
	for i := range nodeTaints {
		if taint.MatchTaint(&nodeTaints[i]) {
			if equality.Semantic.DeepEqual(taint, nodeTaints[i]) {
				return
			}
			newTaints = append(newTaints, taint)
			updated = true
			continue
		}

		newTaints = append(newTaints, nodeTaints[i])
	}

	if !updated {
		newTaints = append(newTaints, taint)
	}

	newNode.Spec.Taints = newTaints

	_, err = clientset.CoreV1().Nodes().Update(newNode)
	if err != nil {
		log.Error(err)
	}
}

func kubeClient(config *rest.Config) *kubernetes.Clientset {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf(err.Error())
	}

	return clientset
}
