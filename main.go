package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	instance_cost = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "intance_hourly_cost_usd",
		Namespace: "AwsCostExporter",
		Help:      "Hourly cost of instance, in USD",
	}, []string{"node", "instance_type", "is_spot"})
	instance_per_core_cost = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "intance_per_core_hourly_cost_usd",
		Namespace: "AwsCostExporter",
		Help:      "Hourly cost of instance, per CPU core, in USD",
	}, []string{"node", "instance_type", "is_spot"})
	instance_per_1g_ram_cost = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "intance_per_1g_ram_hourly_cost_usd",
		Namespace: "AwsCostExporter",
		Help:      "Hourly cost of instance, per 1G RAM, in USD",
	}, []string{"node", "instance_type", "is_spot"})
)

func KubeInit() {
	//TODO support in cluster auth
	var kubeconfig *string
	if home := os.Getenv("HOME"); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	// create the clientset
	kubeClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
}

var InstanceTypePriceCache map[string]InstanceTypePrice
var PricingClient *pricing.Pricing
var ec2Client *ec2.EC2
var kubeClient *kubernetes.Clientset

func main() {
	log.Print("Staring")
	KubeInit()
	AWSInit()

	InstanceTypePriceCache = make(map[string]InstanceTypePrice)
	var iIsSpot bool

	ticker := time.NewTicker(30 * time.Second)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				log.Print(t)
				nodes, err := kubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
				if err != nil {
					panic(err.Error())
				} else {
					log.Println("pulled node list from k8s, found " + strconv.Itoa(len(nodes.Items)))
				}
				for _, node := range nodes.Items {
					iType := node.Labels["beta.kubernetes.io/instance-type"]
					iAZ := node.Labels["failure-domain.beta.kubernetes.io/zone"]
					if node.Labels["node-role.kubernetes.io/spot-worker"] == "true" {
						iIsSpot = true
					} else {
						iIsSpot = false
					}
					iCPU, _ := node.Status.Capacity.Cpu().AsInt64()
					iMemory, _ := node.Status.Capacity.Memory().AsInt64()
					iPrice := GetInstancePrice(iType, iAZ, iIsSpot, iCPU, iMemory)
					log.Println(node.Name)
					instance_cost.With(prometheus.Labels{"node": node.Name, "instance_type": iType, "is_spot": strconv.FormatBool(iIsSpot)}).Set(iPrice.HourlyPrice)
					instance_per_core_cost.With(prometheus.Labels{"node": node.Name, "instance_type": iType, "is_spot": strconv.FormatBool(iIsSpot)}).Set(iPrice.HourlyPricePerCpuCore)
					instance_per_1g_ram_cost.With(prometheus.Labels{"node": node.Name, "instance_type": iType, "is_spot": strconv.FormatBool(iIsSpot)}).Set(iPrice.HourlyPricePerMemKb * 1024 * 1024 * 1024)
				}
				// TODO remove old non active nodes ( speedup "enrichment phase and just use instance_cost.Reset() or compare updated list to whats in metric reg'
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
