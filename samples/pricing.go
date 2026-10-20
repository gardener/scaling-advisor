// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package samples

import (
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
)

// GetTop20AWSInstancePrices returns a sample slice of InstancePrice for the top 20 common AWS EC2 instance types
// in the us-east-1 region. Intended for use in unit tests.
func GetTop20AWSInstancePrices() []sacorev1alpha1.InstancePrice {
	return []sacorev1alpha1.InstancePrice{
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "m5.large"}, Price: "0.096"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "m5.xlarge"}, Price: "0.192"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "m5.2xlarge"}, Price: "0.384"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "m5.4xlarge"}, Price: "0.768"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "m5.8xlarge"}, Price: "1.536"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "c5.large"}, Price: "0.085"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "c5.xlarge"}, Price: "0.170"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "c5.2xlarge"}, Price: "0.340"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "c5.4xlarge"}, Price: "0.680"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "c5.9xlarge"}, Price: "1.530"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "r5.large"}, Price: "0.126"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "r5.xlarge"}, Price: "0.252"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "r5.2xlarge"}, Price: "0.504"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "r5.4xlarge"}, Price: "1.008"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "r5.8xlarge"}, Price: "2.016"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "t3.medium"}, Price: "0.0416"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "t3.large"}, Price: "0.0832"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "t3.xlarge"}, Price: "0.1664"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "t3.2xlarge"}, Price: "0.3328"},
		{InstancePriceKey: sacorev1alpha1.InstancePriceKey{Region: "us-east-1", InstanceType: "p3.2xlarge"}, Price: "3.060"},
	}
}
