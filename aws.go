package main

import (
	"fmt"
	"log"
	"strconv"

	awsPricingTyper "github.com/Oded-B/aws-pricing-typer"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/pricing"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func GetTermKey(m map[string]awsPricingTyper.OnDemandTerm) string {
	for k := range m {
		return k
	}
	return ""
}

func GetPriceDimensionKey(m map[string]awsPricingTyper.PriceDimensionItem) string {
	for k := range m {
		return k
	}
	return ""
}

// func GetOnDemandPrice(region string, instanceType string, az string) {
func GetOnDemandPrice(pc *pricing.Pricing, instanceTypeValue string) float64 {

	// create request criteria
	ec2ServiceCode := "AmazonEC2"
	formatVer := "aws_v1"
	typeTerm := pricing.FilterTypeTermMatch

	// create filters
	locationKey := "location"
	locationValue := "EU (Ireland)"
	instanceTypeKey := "instanceType"
	// instanceTypeValue := "m4.16xlarge"
	instanceOsKey := "operatingSystem"
	instanceOsValue := "Linux"
	csKey := "capacitystatus"
	csValue := "Used"
	opKey := "operation"
	opValue := "RunInstances"
	tenancyKey := "tenancy"
	tenancyValue := "Shared"
	var priceFilters []*pricing.Filter
	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &locationKey,
		Value: &locationValue,
	})
	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &instanceTypeKey,
		Value: &instanceTypeValue,
	})

	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &instanceOsKey,
		Value: &instanceOsValue,
	})
	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &csKey,
		Value: &csValue,
	})
	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &opKey,
		Value: &opValue,
	})
	priceFilters = append(priceFilters, &pricing.Filter{
		Type:  &typeTerm,
		Field: &tenancyKey,
		Value: &tenancyValue,
	})
	// make request
	productsOutput, err := pc.GetProducts(&pricing.GetProductsInput{
		ServiceCode:   &ec2ServiceCode,
		FormatVersion: &formatVer,
		Filters:       priceFilters,
	})
	if err != nil {
		log.Println(err)
	}

	priceData, err := awsPricingTyper.GetTypedPricingData(*productsOutput)
	if err != nil {
		log.Println(err)
	}

	termKey := GetTermKey(priceData[0].Terms.OnDemand)
	priceDimensionsKey := GetPriceDimensionKey(priceData[0].Terms.OnDemand[termKey].PriceDimensions[0])
	return priceData[0].Terms.OnDemand[termKey].PriceDimensions[0][priceDimensionsKey].PricePerUnit[0]["USD"]
}

func GetLastSpotPrice(ec2c *ec2.EC2, region string, instanceType string, az string) float64 {

	input := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes: []*string{
			aws.String(instanceType),
		},
		ProductDescriptions: []*string{
			aws.String("Linux/UNIX (Amazon VPC)"),
		},
		MaxResults:       aws.Int64(1),
		AvailabilityZone: aws.String(az),
	}

	result, err := ec2c.DescribeSpotPriceHistory(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	} else {
		if price, err := strconv.ParseFloat(*result.SpotPriceHistory[0].SpotPrice, 64); err == nil {
			return price
		} else {
			fmt.Println(err)
			return 8888
			// TODO WTFFF
		}
	}
	// TODO WTFFF
	return 99999
}

type InstanceTypePrice struct {
	name                  string
	az                    string
	HourlyPrice           float64
	HourlyPricePerCpuCore float64
	HourlyPricePerMemKb   float64
}

func GetInstancePrice(iType string, iAz string, isSpot bool, cpu int64, memory int64) InstanceTypePrice {
	//TODO  cache invalidation  - time based, diffrent for Spot and OnDemand?

	var cacheKey string

	if isSpot {
		cacheKey = "spot-" + iType + "_" + iAz
	} else {
		cacheKey = "OnDemand-" + iType
	}
	var val InstanceTypePrice
	var found bool

	val, found = InstanceTypePriceCache[cacheKey]
	if found {

		// log.Println("found " + cacheKey + " in cache")
	} else {
		// log.Println("didn't find " + cacheKey + " in cache")
		val = InstanceTypePrice{
			name: iType,
			az:   iAz,
		}

		if isSpot {
			// log.Println("checking spot price for " + iType + " @ " + iAz)
			val.HourlyPrice = GetLastSpotPrice(ec2Client, "us-east-1", iType, iAz)
		} else {
			// log.Println("checking OnDemand price for " + iType)
			val.HourlyPrice = GetOnDemandPrice(PricingClient, iType)
		}
		val.HourlyPricePerCpuCore = val.HourlyPrice / float64(cpu)
		val.HourlyPricePerMemKb = val.HourlyPrice / float64(memory)
		InstanceTypePriceCache[cacheKey] = val
	}
	return val
}

func AWSInit() {
	apiRegion := "us-east-1"
	PricingClient = pricing.New(session.New(&aws.Config{Region: aws.String(apiRegion)}))
	ec2Client = ec2.New(session.New(&aws.Config{Region: aws.String(apiRegion)}))
}
