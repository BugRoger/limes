/*******************************************************************************
*
* Copyright 2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package plugins

import (
	"net/http"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/majewsky/schwift"
	"github.com/majewsky/schwift/gopherschwift"
	"github.com/sapcc/limes/pkg/limes"
	"github.com/sapcc/limes/pkg/util"
)

type swiftPlugin struct {
	cfg limes.ServiceConfiguration
}

var swiftResources = []limes.ResourceInfo{
	{
		Name: "capacity",
		Unit: limes.UnitBytes,
	},
}

func init() {
	limes.RegisterQuotaPlugin(func(c limes.ServiceConfiguration, scrapeSubresources map[string]bool) limes.QuotaPlugin {
		return &swiftPlugin{c}
	})
}

//Init implements the limes.QuotaPlugin interface.
func (p *swiftPlugin) Init(provider *gophercloud.ProviderClient) error {
	return nil
}

//ServiceInfo implements the limes.QuotaPlugin interface.
func (p *swiftPlugin) ServiceInfo() limes.ServiceInfo {
	return limes.ServiceInfo{
		Type:        "object-store",
		ProductName: "swift",
		Area:        "storage",
	}
}

//Resources implements the limes.QuotaPlugin interface.
func (p *swiftPlugin) Resources() []limes.ResourceInfo {
	return swiftResources
}

func (p *swiftPlugin) Account(provider *gophercloud.ProviderClient, projectUUID string) (*schwift.Account, error) {
	client, err := openstack.NewObjectStorageV1(provider,
		gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic},
	)
	if err != nil {
		return nil, err
	}
	resellerAccount, err := gopherschwift.Wrap(client)
	if err != nil {
		return nil, err
	}
	//TODO Make Auth prefix configurable
	return resellerAccount.SwitchAccount("AUTH_" + projectUUID), nil
}

//Scrape implements the limes.QuotaPlugin interface.
func (p *swiftPlugin) Scrape(provider *gophercloud.ProviderClient, clusterID, domainUUID, projectUUID string) (map[string]limes.ResourceData, error) {
	account, err := p.Account(provider, projectUUID)
	if err != nil {
		return nil, err
	}

	headers, err := account.Headers()
	if schwift.Is(err, http.StatusNotFound) || schwift.Is(err, http.StatusGone) {
		//Swift account does not exist or was deleted and not yet reaped, but the keystone project exist
		return map[string]limes.ResourceData{
			"capacity": {
				Quota: 0,
				Usage: 0,
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	data := limes.ResourceData{
		Usage: headers.BytesUsed().Get(),
		Quota: int64(headers.BytesUsedQuota().Get()),
	}
	if !headers.BytesUsedQuota().Exists() {
		data.Quota = -1
	}
	return map[string]limes.ResourceData{"capacity": data}, nil
}

//SetQuota implements the limes.QuotaPlugin interface.
func (p *swiftPlugin) SetQuota(provider *gophercloud.ProviderClient, clusterID, domainUUID, projectUUID string, quotas map[string]uint64) error {
	account, err := p.Account(provider, projectUUID)
	if err != nil {
		return err
	}

	headers := schwift.NewAccountHeaders()
	headers.BytesUsedQuota().Set(quotas["capacity"])
	//this header brought to you by https://github.com/sapcc/swift-addons
	headers.Set("X-Account-Project-Domain-Id-Override", domainUUID)

	err = account.Update(headers, nil)
	if schwift.Is(err, http.StatusNotFound) && quotas["capacity"] > 0 {
		//account does not exist yet - if there is a non-zero quota, enable it now
		err = account.Create(headers.ToOpts())
		if err == nil {
			util.LogInfo("Swift Account %s created", projectUUID)
		}
	}
	return err
}
