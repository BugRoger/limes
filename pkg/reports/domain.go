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

package reports

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sapcc/limes/pkg/db"
	"github.com/sapcc/limes/pkg/limes"
	"github.com/sapcc/limes/pkg/util"
)

//Domain contains aggregated data about resource usage in a domain.
type Domain struct {
	UUID     string         `json:"id"`
	Services DomainServices `json:"services,keepempty"`
}

//DomainService is a substructure of Domain containing data for
//a single backend service.
type DomainService struct {
	Type         string          `json:"type"`
	Resources    DomainResources `json:"resources,keepempty"`
	MaxScrapedAt int64           `json:"max_scraped_at"`
	MinScrapedAt int64           `json:"min_scraped_at"`
}

//DomainResource is a substructure of Domain containing data for
//a single resource.
type DomainResource struct {
	Name                 string     `json:"type"`
	Unit                 limes.Unit `json:"unit,omitempty"`
	DomainQuota          uint64     `json:"quota,keepempty"`
	ProjectsQuota        uint64     `json:"projects_quota,keepempty"`
	Usage                uint64     `json:"usage,,keepemptykeepempty"`
	BackendQuota         *uint64    `json:"backend_quota,omitempty"`
	InfiniteBackendQuota *bool      `json:"infinite_backend_quota,omitempty"`
}

//DomainServices provides fast lookup of services using a map, but serializes
//to JSON as a list.
type DomainServices map[string]*DomainService

//MarshalJSON implements the json.Marshaler interface.
func (s DomainServices) MarshalJSON() ([]byte, error) {
	list := make([]*DomainService, 0, len(s))
	for _, srv := range s {
		list = append(list, srv)
	}
	return json.Marshal(list)
}

//DomainResources provides fast lookup of resources using a map, but serializes
//to JSON as a list.
type DomainResources map[string]*DomainResource

//MarshalJSON implements the json.Marshaler interface.
func (r DomainResources) MarshalJSON() ([]byte, error) {
	list := make([]*DomainResource, 0, len(r))
	for _, res := range r {
		list = append(list, res)
	}
	return json.Marshal(list)
}

var domainReportQuery1 = `
	SELECT d.uuid, ps.type, pr.name, SUM(pr.quota), SUM(pr.usage),
	       SUM(GREATEST(pr.backend_quota, 0)), MIN(pr.backend_quota) < 0, MIN(ps.scraped_at), MAX(ps.scraped_at)
	  FROM domains d
	  JOIN projects p ON p.domain_id = d.id
	  JOIN project_services ps ON ps.project_id = p.id
	  JOIN project_resources pr ON pr.service_id = ps.id
	 WHERE %s GROUP BY d.uuid, ps.type, pr.name
`

var domainReportQuery2 = `
	SELECT d.uuid, ds.type, dr.name, dr.quota
	  FROM domains d
	  JOIN domain_services ds ON ds.domain_id = d.id
	  JOIN domain_resources dr ON dr.service_id = ds.id
	 WHERE %s
`

//GetDomains returns Domain reports for all domains in the given cluster or, if
//domainID is non-nil, for that domain only.
func GetDomains(cluster *limes.ClusterConfiguration, domainID *int64, dbi db.Interface) ([]*Domain, error) {
	fields := map[string]interface{}{"d.cluster_id": cluster.ID}
	if domainID != nil {
		fields["d.id"] = *domainID
	}

	//first query: data for projects in this domain
	whereStr, queryArgs := db.BuildSimpleWhereClause(fields)
	rows, err := dbi.Query(fmt.Sprintf(domainReportQuery1, whereStr), queryArgs...)
	if err != nil {
		return nil, err
	}

	domains := make(map[string]*Domain)
	for rows.Next() {
		var (
			domainUUID           string
			serviceType          string
			resourceName         string
			projectsQuota        uint64
			usage                uint64
			backendQuota         uint64
			infiniteBackendQuota bool
			minScrapedAt         util.Time
			maxScrapedAt         util.Time
		)
		err := rows.Scan(
			&domainUUID, &serviceType, &resourceName,
			&projectsQuota, &usage, &backendQuota, &infiniteBackendQuota,
			&minScrapedAt, &maxScrapedAt,
		)
		if err != nil {
			rows.Close()
			return nil, err
		}

		domain, exists := domains[domainUUID]
		if !exists {
			domain = &Domain{
				UUID:     domainUUID,
				Services: make(DomainServices),
			}
			domains[domainUUID] = domain
		}

		service, exists := domain.Services[serviceType]
		if !exists {
			service = &DomainService{
				Type:         serviceType,
				Resources:    make(DomainResources),
				MaxScrapedAt: time.Time(maxScrapedAt).Unix(),
				MinScrapedAt: time.Time(minScrapedAt).Unix(),
			}
			domain.Services[serviceType] = service
		}

		resource := &DomainResource{
			Name:                 resourceName,
			Unit:                 limes.UnitFor(serviceType, resourceName),
			DomainQuota:          0, //will be measured in the next step
			ProjectsQuota:        projectsQuota,
			Usage:                usage,
			BackendQuota:         nil, //see below
			InfiniteBackendQuota: nil, //see below
		}
		if projectsQuota != backendQuota {
			resource.BackendQuota = &backendQuota
		}
		if infiniteBackendQuota {
			resource.InfiniteBackendQuota = &infiniteBackendQuota
		}
		service.Resources[resourceName] = resource
	}
	err = rows.Err()
	if err != nil {
		rows.Close()
		return nil, err
	}
	err = rows.Close()
	if err != nil {
		return nil, err
	}

	//second query: add domain quotas
	rows, err = dbi.Query(fmt.Sprintf(domainReportQuery2, whereStr), queryArgs...)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			domainUUID   string
			serviceType  string
			resourceName string
			quota        uint64
		)
		err := rows.Scan(
			&domainUUID, &serviceType, &resourceName, &quota,
		)
		if err != nil {
			rows.Close()
			return nil, err
		}

		domain, exists := domains[domainUUID]
		if !exists {
			domain = &Domain{
				UUID:     domainUUID,
				Services: make(DomainServices),
			}
			domains[domainUUID] = domain
		}

		service, exists := domain.Services[serviceType]
		if !exists {
			service = &DomainService{
				Type:      serviceType,
				Resources: make(DomainResources),
			}
			domain.Services[serviceType] = service
		}

		resource, exists := service.Resources[resourceName]
		if exists {
			resource.DomainQuota = quota
		} else {
			resource = &DomainResource{
				Name:        resourceName,
				Unit:        limes.UnitFor(serviceType, resourceName),
				DomainQuota: quota,
			}
			service.Resources[resourceName] = resource
		}
	}
	err = rows.Err()
	if err != nil {
		rows.Close()
		return nil, err
	}
	err = rows.Close()
	if err != nil {
		return nil, err
	}

	//validate against known services/resources
	isValidService := make(map[string]bool)
	for _, srv := range cluster.Services {
		isValidService[srv.Type] = true
	}

	for _, domain := range domains {
		for serviceType, service := range domain.Services {
			if !isValidService[serviceType] {
				delete(domain.Services, serviceType)
				continue
			}

			isValidResource := make(map[string]bool)
			if plugin := limes.GetQuotaPlugin(serviceType); plugin != nil {
				for _, res := range plugin.Resources() {
					isValidResource[res.Name] = true
				}
			}

			for resourceName := range service.Resources {
				if !isValidResource[resourceName] {
					delete(service.Resources, resourceName)
				}
			}
		}
	}

	//flatten result
	result := make([]*Domain, 0, len(domains))
	for _, domain := range domains {
		result = append(result, domain)
	}

	return result, nil
}