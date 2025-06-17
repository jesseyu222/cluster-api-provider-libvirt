// SPDX-License-Identifier: Apache-2.0
// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
    "crypto/tls"
    "flag"
    "os"
    "path/filepath"

    _ "k8s.io/client-go/plugin/pkg/client/auth" // auth providers

    "k8s.io/apimachinery/pkg/runtime"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/certwatcher"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    "sigs.k8s.io/controller-runtime/pkg/webhook"

    // Cluster API core types (Cluster/Machine, etc.)
    clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

    // Provider API types
    infrav1 "github.com/jesseyu222/cluster-api-provider-libvirt/api/v1beta1"

    // Provider controllers
    "github.com/jesseyu222/cluster-api-provider-libvirt/internal/controller"
    // +kubebuilder:scaffold:imports
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    // Kubernetes built‑in types
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))

    // Cluster API core contract (v1beta1)
    utilruntime.Must(clusterv1.AddToScheme(scheme))

    // Provider’s own API
    utilruntime.Must(infrav1.AddToScheme(scheme))

    // +kubebuilder:scaffold:scheme
}

func main() { //nolint:gocyclo
    var (
        metricsAddr                                               string
        metricsCertPath, metricsCertName, metricsCertKey          string
        webhookCertPath, webhookCertName, webhookCertKey          string
        enableLeaderElection, secureMetrics, enableHTTP2          bool
        probeAddr                                                 string
        tlsOpts                                                   []func(*tls.Config)
    )

    flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to (':8080' for HTTP, ':8443' for HTTPS, '0' to disable)")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for the controller manager.")
    flag.BoolVar(&secureMetrics, "metrics-secure", false, "Serve metrics over HTTPS if true.")
    flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "Directory containing the webhook TLS cert & key.")
    flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "Webhook TLS certificate filename.")
    flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "Webhook TLS key filename.")
    flag.StringVar(&metricsCertPath, "metrics-cert-path", "", "Directory containing the metrics TLS cert & key.")
    flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "Metrics TLS certificate filename.")
    flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "Metrics TLS key filename.")
    flag.BoolVar(&enableHTTP2, "enable-http2", false, "Enable HTTP/2 for the metrics & webhook servers (disabled by default due to CVEs).")

    opts := zap.Options{Development: true}
    opts.BindFlags(flag.CommandLine)
    flag.Parse()
    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    // Disable HTTP/2 unless explicitly requested
    if !enableHTTP2 {
        tlsOpts = append(tlsOpts, func(c *tls.Config) {
            setupLog.Info("disabling http/2 on servers")
            c.NextProtos = []string{"http/1.1"}
        })
    }

    // ----- Cert watchers -----
    var (
        metricsCertWatcher *certwatcher.CertWatcher
        webhookCertWatcher *certwatcher.CertWatcher
        webhookTLSOpts     = tlsOpts
    )

    if webhookCertPath != "" {
        w, err := certwatcher.New(
            filepath.Join(webhookCertPath, webhookCertName),
            filepath.Join(webhookCertPath, webhookCertKey),
        )
        if err != nil {
            setupLog.Error(err, "failed to initialise webhook cert watcher")
            os.Exit(1)
        }
        webhookCertWatcher = w
        webhookTLSOpts = append(webhookTLSOpts, func(c *tls.Config) {
            c.GetCertificate = webhookCertWatcher.GetCertificate
        })
    }

    if metricsCertPath != "" {
        m, err := certwatcher.New(
            filepath.Join(metricsCertPath, metricsCertName),
            filepath.Join(metricsCertPath, metricsCertKey),
        )
        if err != nil {
            setupLog.Error(err, "failed to initialise metrics cert watcher")
            os.Exit(1)
        }
        metricsCertWatcher = m
        tlsOpts = append(tlsOpts, func(c *tls.Config) {
            c.GetCertificate = metricsCertWatcher.GetCertificate
        })
    }

    // ----- Servers -----
    webhookServer := webhook.NewServer(webhook.Options{TLSOpts: webhookTLSOpts})

    metricsOpts := metricsserver.Options{
        BindAddress:   metricsAddr,
        SecureServing: secureMetrics,
        TLSOpts:       tlsOpts,
    }
    if secureMetrics {
        metricsOpts.FilterProvider = filters.WithAuthenticationAndAuthorization
    }

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                 scheme,
        Metrics:                metricsOpts,
        WebhookServer:          webhookServer,
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "capi-libvirt-manager-leader-election",
    })
    if err != nil {
        setupLog.Error(err, "unable to create manager")
        os.Exit(1)
    }

    // ----- Reconcilers -----
    if err := (&controller.LibvirtClusterReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create LibvirtCluster controller")
        os.Exit(1)
    }
    if err := (&controller.LibvirtMachineReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create LibvirtMachine controller")
        os.Exit(1)
    }
    if err := (&controller.LibvirtMachineTemplateReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create LibvirtMachineTemplate controller")
        os.Exit(1)
    }
    // +kubebuilder:scaffold:builder

    // ----- Add cert watchers -----
    if metricsCertWatcher != nil {
        if err := mgr.Add(metricsCertWatcher); err != nil {
            setupLog.Error(err, "unable to add metrics cert watcher")
            os.Exit(1)
        }
    }
    if webhookCertWatcher != nil {
        if err := mgr.Add(webhookCertWatcher); err != nil {
            setupLog.Error(err, "unable to add webhook cert watcher")
            os.Exit(1)
        }
    }

    // ----- Health checks -----
    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up health check")
        os.Exit(1)
    }
    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up ready check")
        os.Exit(1)
    }

    setupLog.Info("starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
