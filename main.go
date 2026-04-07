package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/vadimkim/cert-manager-webhook-hetzner/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&hetznerDNSProviderSolver{},
	)
}

type hetznerDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type hetznerDNSProviderConfig struct {
	SecretName string `json:"secretName"`
	SecretKey  string `json:"secretKey"`
	ZoneName   string `json:"zoneName"`
	ApiUrl     string `json:"apiUrl"`
}

func (c *hetznerDNSProviderSolver) Name() string {
	return "hetzner"
}

func (c *hetznerDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.V(6).Infof("call function Present: namespace=%s, zone=%s, fqdn=%s",
		ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	config, err := c.buildConfig(ch)
	if err != nil {
		return err
	}

	rrsetName := recordName(ch.ResolvedFQDN, config.ZoneName)

	existing, err := getRRSet(config, rrsetName)
	if err != nil {
		return fmt.Errorf("unable to check existing RRSet: %v", err)
	}

	if existing == nil {
		ttl := internal.DefaultTxtTTL
		createReq := internal.RRSetCreateRequest{
			Name: rrsetName,
			Type: "TXT",
			TTL:  &ttl,
			Records: []internal.RRSetRecord{
				{Value: ch.Key},
			},
		}
		body, err := json.Marshal(createReq)
		if err != nil {
			return fmt.Errorf("unable to marshal create request: %v", err)
		}

		url := config.ApiUrl + "/zones/" + config.ZoneIdStr() + "/rrsets"
		_, err = callApi(url, "POST", bytes.NewReader(body), config)
		if err != nil {
			return fmt.Errorf("unable to create TXT RRSet: %v", err)
		}
		klog.Infof("Created TXT RRSet %s", rrsetName)
	} else {
		addReq := internal.RRSetAddRecordsRequest{
			Records: []internal.RRSetRecord{
				{Value: ch.Key},
			},
		}
		body, err := json.Marshal(addReq)
		if err != nil {
			return fmt.Errorf("unable to marshal add records request: %v", err)
		}

		url := config.ApiUrl + "/zones/" + config.ZoneIdStr() + "/rrsets/" + rrsetName + "/TXT/actions/add_records"
		_, err = callApi(url, "POST", bytes.NewReader(body), config)
		if err != nil {
			return fmt.Errorf("unable to add record to TXT RRSet: %v", err)
		}
		klog.Infof("Added record to existing TXT RRSet %s", rrsetName)
	}

	return nil
}

func (c *hetznerDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	config, err := c.buildConfig(ch)
	if err != nil {
		return err
	}

	rrsetName := recordName(ch.ResolvedFQDN, config.ZoneName)

	existing, err := getRRSet(config, rrsetName)
	if err != nil {
		return fmt.Errorf("unable to get TXT RRSet for cleanup: %v", err)
	}
	if existing == nil {
		klog.Infof("RRSet %s already gone, nothing to clean up", rrsetName)
		return nil
	}

	if len(existing.Records) <= 1 {
		url := config.ApiUrl + "/zones/" + config.ZoneIdStr() + "/rrsets/" + rrsetName + "/TXT"
		_, err = callApi(url, "DELETE", nil, config)
		if err != nil {
			return fmt.Errorf("unable to delete TXT RRSet: %v", err)
		}
		klog.Infof("Deleted TXT RRSet %s", rrsetName)
	} else {
		removeReq := internal.RRSetRemoveRecordsRequest{
			Records: []internal.RRSetRecord{
				{Value: ch.Key},
			},
		}
		body, err := json.Marshal(removeReq)
		if err != nil {
			return fmt.Errorf("unable to marshal remove records request: %v", err)
		}

		url := config.ApiUrl + "/zones/" + config.ZoneIdStr() + "/rrsets/" + rrsetName + "/TXT/actions/remove_records"
		_, err = callApi(url, "POST", bytes.NewReader(body), config)
		if err != nil {
			return fmt.Errorf("unable to remove record from TXT RRSet: %v", err)
		}
		klog.Infof("Removed record from TXT RRSet %s", rrsetName)
	}

	return nil
}

func (c *hetznerDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	k8sClient, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = k8sClient

	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (hetznerDNSProviderConfig, error) {
	cfg := hetznerDNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func stringFromSecretData(secretData map[string][]byte, key string) (string, error) {
	data, ok := secretData[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

func (c *hetznerDNSProviderSolver) buildConfig(ch *v1alpha1.ChallengeRequest) (internal.Config, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return internal.Config{}, err
	}

	apiUrl := cfg.ApiUrl
	if apiUrl == "" {
		apiUrl = internal.DefaultApiUrl
	}

	secretKey := cfg.SecretKey
	if secretKey == "" {
		secretKey = internal.DefaultSecretKey
	}

	secretName := cfg.SecretName
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return internal.Config{}, fmt.Errorf("unable to get secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	apiKey, err := stringFromSecretData(sec.Data, secretKey)
	if err != nil {
		return internal.Config{}, fmt.Errorf("unable to get key %q from secret `%s/%s`; %v", secretKey, secretName, ch.ResourceNamespace, err)
	}

	config := internal.Config{
		ApiKey:    apiKey,
		ApiUrl:    apiUrl,
		SecretKey: secretKey,
		ZoneName:  cfg.ZoneName,
	}

	if config.ZoneName == "" {
		foundZone, err := searchZoneName(config, ch.ResolvedZone)
		if err != nil {
			return config, err
		}
		config.ZoneName = foundZone
	}

	zoneId, err := searchZoneId(config)
	if err != nil {
		return config, fmt.Errorf("unable to find zone id for `%s`: %v", config.ZoneName, err)
	}
	config.ZoneId = zoneId

	return config, nil
}

func recordName(fqdn, domain string) string {
	r := regexp.MustCompile("(.+)\\." + domain + "\\.")
	name := r.FindStringSubmatch(fqdn)
	if len(name) != 2 {
		klog.Errorf("splitting domain name %s failed!", fqdn)
		return ""
	}
	return name[1]
}

func callApi(url, method string, body io.Reader, config internal.Config) ([]byte, error) {
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.ApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Fatal(err)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respBody, nil
	}

	text := fmt.Sprintf("Error calling API status: %s url: %s method: %s body: %s", resp.Status, url, method, string(respBody))
	klog.Error(text)
	return nil, errors.New(text)
}

func searchZoneId(config internal.Config) (int64, error) {
	url := config.ApiUrl + "/zones?name=" + config.ZoneName

	data, err := callApi(url, "GET", nil, config)
	if err != nil {
		return 0, fmt.Errorf("unable to get zone info: %v", err)
	}

	zones := internal.ZoneListResponse{}
	if err := json.Unmarshal(data, &zones); err != nil {
		return 0, fmt.Errorf("unable to unmarshal zone response: %v", err)
	}

	if len(zones.Zones) != 1 {
		return 0, fmt.Errorf("wrong number of zones in response %d, expected exactly 1", len(zones.Zones))
	}

	klog.V(6).Infof("Found zone %s with id %d", config.ZoneName, zones.Zones[0].Id)
	return zones.Zones[0].Id, nil
}

func getRRSet(config internal.Config, rrsetName string) (*internal.RRSet, error) {
	url := config.ApiUrl + "/zones/" + config.ZoneIdStr() + "/rrsets/" + rrsetName + "/TXT"

	data, err := callApi(url, "GET", nil, config)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, err
	}

	resp := internal.RRSetResponse{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unable to unmarshal RRSet response: %v", err)
	}

	return &resp.RRSet, nil
}

func searchZoneName(config internal.Config, searchZone string) (string, error) {
	parts := strings.Split(searchZone, ".")
	parts = parts[:len(parts)-1]
	for i := 0; i <= len(parts)-2; i++ {
		candidate := strings.Join(parts[i:], ".")
		config.ZoneName = candidate
		zoneId, err := searchZoneId(config)
		if err == nil && zoneId != 0 {
			klog.Infof("Found zone with name: %s (id: %s)", candidate, strconv.FormatInt(zoneId, 10))
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to find hetzner dns zone with: %s", searchZone)
}
