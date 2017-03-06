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

package models

import (
	"github.com/sapcc/limes/pkg/drivers"
	"github.com/sapcc/limes/pkg/limes"
)

//Project represents a Keystone project in Limes' database.
type Project struct {
	ID                      int64
	drivers.KeystoneProject //Name and UUID
	DomainID                int64
}

//ProjectsTable enables table-level operations on projects.
var ProjectsTable = &Table{
	Name:       "projects",
	AllFields:  []string{"id", "domain_id", "uuid", "name"},
	makeRecord: func() Record { return &Project{} },
}

//CreateProject puts a new project in the database.
func CreateProject(kp drivers.KeystoneProject, domainID int64, db DBInterface) (*Project, error) {
	p := &Project{
		KeystoneProject: kp,
		DomainID:        domainID,
	}
	return p, db.QueryRow(
		`INSERT INTO projects (domain_id, uuid, name) VALUES ($1, $2, $3) RETURNING id`,
		p.DomainID, p.UUID, p.Name,
	).Scan(&p.ID)
}

//Table implements the Record interface.
func (p *Project) Table() *Table {
	return ProjectsTable
}

//ScanTargets implements the Record interface.
func (p *Project) ScanTargets() []interface{} {
	return []interface{}{
		&p.ID, &p.DomainID, &p.UUID, &p.Name,
	}
}

//Delete implements the Record interface.
func (p *Project) Delete() error {
	_, err := limes.DB.Exec(`DELETE FROM projects WHERE id = $1`, p.ID)
	return err
}