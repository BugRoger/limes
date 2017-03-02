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

package collectors

import (
	"github.com/sapcc/limes/pkg/drivers"
	"github.com/sapcc/limes/pkg/models"
)

//ScanDomainsOpts contains additional options for ScanDomains().
type ScanDomainsOpts struct {
	//Recurse into ScanProjects for all domains in the selected cluster,
	//rather than just for new domains.
	ScanAllProjects bool
}

//ScanDomains queries Keystone to discover new domains, and returns a
//list of UUIDs for the newly discovered domains.
func ScanDomains(driver drivers.Driver, clusterID string, opts ScanDomainsOpts) ([]string, error) {
	//list domains in Keystone
	domains, err := driver.ListDomains()
	if err != nil {
		return nil, err
	}
	isDomainUUID := make(map[string]bool)
	for _, domain := range domains {
		isDomainUUID[domain.UUID] = true
	}

	//when a domain has been deleted in Keystone, remove it from our database,
	//too (the deletion from the `domains` table includes all projects in that
	//domain and to all related resource records through `ON DELETE CASCADE`)
	isDomainUUIDinDB := make(map[string]bool)
	var dbDomains []*models.Domain
	err = models.DomainsTable.WalkWhere(`cluster_id = $1`, []interface{}{clusterID},
		func(record models.Record) error {
			domain := record.(*models.Domain)
			if !isDomainUUID[domain.UUID] {
				return domain.Delete()
			}
			isDomainUUIDinDB[domain.UUID] = true
			dbDomains = append(dbDomains, domain)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	//when a domain has been created in Keystone, create the corresponding record
	//in our DB and scan its projects immediately
	var result []string
	for _, domain := range domains {
		if isDomainUUIDinDB[domain.UUID] {
			continue
		}

		dbDomain, err := models.CreateDomain(domain, clusterID)
		if err != nil {
			return result, err
		}
		result = append(result, domain.UUID)

		//with ScanAllProjects = true, we will scan projects in the next step, so skip now
		if !opts.ScanAllProjects {
			_, err = ScanProjects(driver, dbDomain)
			if err != nil {
				return result, err
			}
		}
	}

	//recurse into ScanProjects if requested
	if opts.ScanAllProjects {
		for _, dbDomain := range dbDomains {
			_, err = ScanProjects(driver, dbDomain)
			if err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

//ScanProjects queries Keystone to discover new projects in the given domain.
func ScanProjects(driver drivers.Driver, domain *models.Domain) ([]string, error) {
	//list projects in Keystone
	projects, err := driver.ListProjects(domain.UUID)
	if err != nil {
		return nil, err
	}
	isProjectUUID := make(map[string]bool)
	for _, project := range projects {
		isProjectUUID[project.UUID] = true
	}

	//when a project has been deleted in Keystone, remove it from our database,
	//too (the deletion from the `projects` table includes the projects' resource
	//records through `ON DELETE CASCADE`)
	isProjectUUIDinDB := make(map[string]bool)
	err = models.ProjectsTable.WalkWhere(`domain_id = $1`, []interface{}{domain.ID},
		func(project models.Record) error {
			uuid := project.(*models.Project).UUID
			if !isProjectUUID[uuid] {
				return project.Delete()
			}
			isProjectUUIDinDB[uuid] = true
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	//when a project has been created in Keystone, create the corresponding
	//record in our DB
	var result []string
	for _, project := range projects {
		if isProjectUUIDinDB[project.UUID] {
			continue
		}

		_, err := models.CreateProject(project, domain.ID)
		if err != nil {
			return result, err
		}
		result = append(result, project.UUID)
	}

	return result, nil
}
