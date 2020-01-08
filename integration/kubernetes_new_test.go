package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/containous/traefik/v2/pkg/config/dynamic"
	// "github.com/cenkalti/backoff/v3"
	"github.com/go-check/check"
	checker "github.com/vdemeester/shakers"
)

// KubernetesNewSuite
type KubernetesNewSuite struct{ BaseSuite }

func (s *KubernetesNewSuite) SetUpSuite(c *check.C) {
	s.startk3s(c)
	s.startAndWaitForCoreDNS(c)
	s.startWhoami(c)
}

func (s *KubernetesNewSuite) TearDownSuite(c *check.C) {
	s.stopK3s()
}

func (s *KubernetesNewSuite) TestHTTP(c *check.C) {
	cmd := s.startMaeshBinaryCmd(c)
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer s.stopMaeshBinary(c, cmd.Process)

	var config *dynamic.Configuration

	testFunc := func() error {
		url := fmt.Sprintf("http://127.0.0.1:%d/api/configuration/current", maeshAPIPort)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("status was not ok: %d", resp.StatusCode)
			}

			bodyData, bodyErr := ioutil.ReadAll(resp.Body)
			if bodyErr != nil {
				return bodyErr
			}

			fmt.Println(string(bodyData))

			jsonErr := json.Unmarshal(bodyData, &config)
			if jsonErr != nil {
				return jsonErr
			}

			fmt.Printf("Parsed config: %+v\n\n", config)

		}
		return nil
	}

	c.Assert(s.try.WaitFunction(testFunc, 30*time.Second), checker.IsNil)

}
