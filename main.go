package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

var (
	jsonContentType = `application/json`
	addr            = flag.String("addr", ":8443", "http server address")
	certFile        = flag.String("crt", "server.crt", "certificate file path")
	keyFile         = flag.String("key", "server.key", "certificate key path")
	labelKey        = flag.String("label", "app", "label filter")
	etcdImage       = flag.String("etcd", "quay.io/coreos/etcd", "etcd image url filter")
	optTimeout      = flag.Duration("timeout", 30*time.Second, "remove operation timeout")
	namespace       = flag.String("namespace", "default", "namespace filter")
)

func gracefulRemoveEtcdMember(podName string) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	pod, err := clientset.CoreV1().Pods(*namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	labelValue := pod.Labels[*labelKey]
	pods, err := clientset.CoreV1().Pods(*namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", *labelKey, labelValue),
	})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return errors.New("no matched pods")
	}
	targetPort := int32(2379)
	urls := []string{}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			if strings.HasPrefix(c.Image, *etcdImage) {
				for _, port := range c.Ports {
					if port.Name == "client" {
						targetPort = port.ContainerPort
						url := fmt.Sprintf("%s.%s.%s.svc:%d", pod.Name, labelValue, pod.Namespace, targetPort)
						urls = append(urls, url)
					}
				}
			}
		}
	}

	cfg := clientv3.Config{
		Endpoints:   urls,
		DialTimeout: *optTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, _ := context.WithTimeout(context.Background(), *optTimeout)
	memberList, err := etcdcli.MemberList(ctx)
	if err != nil {
		return err
	}
	if len(memberList.Members) == 0 {
		return errors.New("non available member in the etcd cluster")
	}
	var pickedMember *etcdserverpb.Member
	for _, m := range memberList.Members {
		if m.Name == podName {
			pickedMember = m
		}
	}
	if pickedMember != nil {
		if _, err := etcdcli.Cluster.MemberRemove(ctx, pickedMember.ID); err != nil {
			log.Printf("remove etcd member occurs error: %s", err)
			return err
		}
		if err := clientset.CoreV1().Pods(*namespace).Delete(podName, metav1.NewDeleteOptions(5)); err != nil {
			log.Printf("clean up pod[%s] occurs error: %s", podName, err)
			return err
		}
		//TODO clean up PVC

		log.Printf("sent memberremove request success: %s", podName)
		return err
	}
	log.Printf("no validated member")
	return nil
}

func isServicAccountsGroup(groups []string) bool {
	for _, g := range groups {
		if g == "system:serviceaccounts" {
			return true
		}
	}
	return false
}

func handleValidationAdmissionControl(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("unsupported content type %s, only %s is supported", contentType, jsonContentType)
		return
	}

	var adReview v1beta1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&adReview); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("could not read request body: %v\n", err)
		return
	}

	//adReviewBytes, _ := json.Marshal(adReview)
	//log.Printf("request.body: \n%s", adReviewBytes)

	admissionResp := v1beta1.AdmissionResponse{
		Allowed: true,
	}
	buf := bytes.Buffer{}
	// only handle the deletion
	if adReview.Request.Operation == v1beta1.Delete &&
		adReview.Request.Kind.Kind == "Pod" && !isServicAccountsGroup(adReview.Request.UserInfo.Groups) {
		log.Printf(string(adReview.Request.Name))

		if err := gracefulRemoveEtcdMember(adReview.Request.Name); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("remove etcd member error: %s", err)

			admissionResp.Allowed = false
			json.NewEncoder(&buf).Encode(&admissionResp)
			if _, err := w.Write(buf.Bytes()); err != nil {
				log.Printf("serialize failed: %v\n", err)
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(&buf).Encode(&admissionResp)
		if _, err := w.Write(buf.Bytes()); err != nil {
			log.Printf("serialize failed: %v\n", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
			cancel()
		}
		return nil
	})

	var httpServer *http.Server
	g.Go(func() error {
		defer log.Print("http server exited")
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleValidationAdmissionControl)

		httpServer = &http.Server{
			Addr:    *addr,
			Handler: mux,
		}
		log.Printf("http server started: %s", *addr)
		if err := httpServer.ListenAndServeTLS(*certFile, *keyFile); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	g.Go(func() error {
		select {
		case <-ctx.Done():
			if httpServer != nil {
				if err := httpServer.Shutdown(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Printf("occurred an error: %s", err)
	}
}
