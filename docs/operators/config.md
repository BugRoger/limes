# Configuration guide

Limes requires a configuration file in [YAML format][yaml]. A minimal complete configuration could look like this:

```yaml
database:
  location: "postgres://postgres@localhost/limes"

api:
  listen: "127.0.0.1:8080"
  policy: "/etc/limes/policy.json"

collector:
  metrics: "127.0.0.1:8081"

clusters:
  staging:
    auth:
      auth_url:            https://keystone.staging.example.com/v3
      user_name:           limes
      user_domain_name:    Default
      project_name:        service
      project_domain_name: Default
      password:            swordfish
    services:
      - type: compute
      - type: network
    capacitors:
      - id: nova
    subresources:
      compute:
        - instances
    subcapacities:
      compute:
        - cores
        - ram
    constraints: /etc/limes/constraints-for-staging.yaml
```

Read on for the full list and description of all configuration options.

## Section "database"

Configuration options relating to the database connection of all services.

| Field | Required | Description |
| --- | --- | --- |
| `database.location` | yes | A [libpq connection URI][pq-uri] that locates the Limes database. The non-URI "connection string" format is not allowed; it must be a URI. |

## Section "api"

Configuration options relating to the behavior of the API service.

| Field | Required | Description |
| --- | --- | --- |
| `api.listen` | yes | Bind address for the HTTP API exposed by this service, e.g. `127.0.0.1:8080` to bind only on one IP, or `:8080` to bind on all interfaces and addresses. |
| `api.policy` | yes | Path to the oslo.policy file that describes authorization behavior for this service. Please refer to the [OpenStack documentation on policies][policy] for syntax reference. This repository includes an [example policy][ex-pol] that can be used for development setups, or as a basis for writing your own policy. |
| `api.request_log.except_status_codes` | no | A list of HTTP status codes for which requests will not be logged. A useful setting is `[300]` when using `GET /` requests as a healthcheck. |

## Section "collector"

Configuration options relating to the behavior of the collector service.

| Field | Required | Description |
| --- | --- | --- |
| `collector.metrics` | yes | Bind address for the Prometheus metrics endpoint provided by this service. See `api.listen` for acceptable values. |
| `collector.data_metrics` | no | If set to `true`, expose all quota/usage/capacity data as Prometheus gauges. This is disabled by default because this can be a lot of data for OpenStack clusters containing many projects, domains and services. |

## Section "clusters"

Configuration options describing the OpenStack clusters which Limes shall cover. `$id` is the internal *cluster ID*, which may be chosen freely, but should not be changed afterwards. (It *can* be changed, but that requires a shutdown of all Limes components and manual editing of the database.)

| Field | Required | Description | Equivalent to |
| --- | --- | --- | :--- |
| `clusters.$id.auth.auth_url` | yes | URL for Keystone v3 API in this cluster. Should end in `/v3`. Other Keystone API versions are not supported. | `$OS_AUTH_URL` |
| `clusters.$id.auth.user_name` | yes | Limes service user. | `OS_USERNAME` |
| `clusters.$id.auth.user_domain_name` | yes | Domain containing Limes service user. | `OS_USER_DOMAIN_NAME` |
| `clusters.$id.auth.project_name` | yes | Project where Limes service user has access. | `OS_PROJECT_NAME` |
| `clusters.$id.auth.project_domain_name` | yes | Domain containing that project. | `OS_PROJECT_DOMAIN_NAME` |
| `clusters.$id.auth.password` | yes | Password for Limes service user. | `OS_PASSWORD` |
| `clusters.$id.auth.region_name` | no | In multi-region OpenStack clusters, this selects the region to work on. | `OS_REGION_NAME` |

| Field | Required | Description |
| --- | --- | --- |
| `clusters.$id.catalog_url` | no | URL of Limes API service as it appears in the Keystone service catalog for this cluster. This is only used for version advertisements, and can be omitted if no client relies on the URLs in these version advertisements. |
| `clusters.$id.discovery.method` | no | Defines which method to use to discover Keystone domains and projects in this cluster. If not given, the default value is `list`. |
| `clusters.$id.discovery.except_domains` | no | May contain a regex. Domains whose names match the regex will not be considered by Limes. |
| `clusters.$id.discovery.only_domains` | no | May contain a regex. If given, only domains whose names match the regex will be considered by Limes. If `except_domains` is also given, it takes precedence over `only_domains`. |
| `clusters.$id.services` | yes | List of backend services for which to scrape quota/usage data. Service types for which Limes does not include a suitable *quota plugin* will be ignored. See below for supported service types. |
| `clusters.$id.subresources` | no | List of resources where subresource scraping is requested. This is an object with service types as keys, and a list of resource names as values. |
| `clusters.$id.subcapacities` | no | List of resources where subcapacity scraping is requested. This is an object with service types as keys, and a list of resource names as values. |
| `clusters.$id.capacitors` | no | List of capacity plugins to use for scraping capacity data. See below for supported capacity plugins. |
| `clusters.$id.authoritative` | no | If set to `true`, the collector will write the quota from its own database into the backend service whenever scraping encounters a backend quota that differs from the expectation. This flag is strongly recommended in production systems to avoid divergence of Limes quotas from backend quotas, but should be used with care during development. |
| `clusters.$id.constraints` | no | Path to a YAML file containing the quota constraints for this cluster. See [*quota constraints*](constraints.md) for details. |

# Supported discovery methods

This section lists all supported discovery methods for Keystone domains and projects.

## Method: `list` (default)

```yaml
discovery:
  method: list
```

When this method is configured, Limes will simply list all Keystone domains and projects with the standard API calls, equivalent to what the CLI commands `openstack domain list` and `openstack project list --domain $DOMAIN_ID` do.

## Method: `role-assignment`

```yaml
discovery:
  method: role-assignment
  role-assignment:
    role: swiftoperator
```

When this method is configured, Limes will only consider those Keystone projects where the configured role is assigned to at least one user or group. Role assignments to domains are not considered.

This method is useful when there are a lot of projects (e.g. more than 10 000), but only a few of them (e.g. a few dozen) need to be considered by Limes. Since this method requires one API call per project (times the number of domains) every three minutes, it is not advisable when a lot of projects need to be considered by Limes.

# Supported service types

This section lists all supported service types and the resources that are understood for each service. The `type` string is always equal to the one that appears in the Keystone service catalog.

For each service, `shared: true` can be set to indicate that the resources of this service are shared among all clusters that specify this service type. For example:

```yaml
services:
  # unshared service
  - type: compute
  # shared service
  - type: object-store
    shared: true
```

For each service, an `auth:` section can be given to provide alternative credentials for operations on this service type
(i.e. get quota/usage, set quota). This is particularly useful for shared services, when the service user with the
required permissions is in a different cluster than the one for which quotas are managed. The structure is the same as
for `clusters.$id.auth`. For example:

```yaml
services:
  - type: compute
  - type: object-store
    shared: true
    auth:
      auth_url:            https://keystone.staging.example.com/v3
      user_name:           limes
      user_domain_name:    Default
      project_name:        service
      project_domain_name: Default
      password:            swordfish
      region_name:         staging
```

## `compute`: Nova v2

```yaml
services:
  - type: compute
```

The area for this service is `compute`.

| Resource | Unit |
| --- | --- |
| `cores` | countable |
| `instances` | countable |
| `ram` | MiB |

The `instances` resource supports subresource scraping. Subresources bear the following attributes:

| Attribute | Type | Comment |
| --- | --- | --- |
| `id` | string | instance UUID |
| `name` | string | instance name |
| `status` | string | instance status [as reported by OpenStack Nova](https://wiki.openstack.org/wiki/VMState) |
| `flavor` | string | flavor name (not ID!) |
| `ram` | integer value with unit | amount of memory configured in flavor |
| `vcpu` | integer | number of vCPUs configured in flavor |
| `disk` | integer value with unit | root disk size configured in flavor |
| `os_type` | string | identifier for OS type as configured in image (see below) |
| `hypervisor` | string | identifier for the hypervisor type of this instance (only if configured, see below) |

The `os_type` field contains:
- for VMware images: the value of the `vmware_ostype` property of the instance's image, or
- otherwise: the part after the colon of a tag starting with `ostype:`, e.g. `rhel` if there is a tag `ostype:rhel` on the image.

The value of the `hypervisor` field is determined by looking at the extra specs of the instance's flavor, using matching
rules supplied in the configuration like this:

```yaml
services:
  - type: compute
    compute:
      hypervisor_type_rules:
        - match: extra-spec:vmware:hv_enabled # match on extra spec with name "vmware:hv_enabled"
          pattern: '^True$'                   # regular expression
          type: vmware
        - match: flavor-name
          pattern: '^kvm'
          type: kvm
        - match: extra-spec:capabilities:cpu_arch
          pattern: '.+'
          type: none # i.e. bare-metal
```

Rules are evaluated in the order given, and the first matching rule will be taken. If no rule matches, the hypervisor
will be reported as `unknown`. If rules cannot be evaluated because the instance's flavor has been deleted, the
hypervisor will be reported as `flavor-deleted`.

On SAP Converged Cloud (or any other OpenStack cluster where Nova carries the relevant patches), there will be an
additional resource `instances_<flavorname>` for each flavor with the `quota:separate = true` extra spec. These resources
behave like the `instances` resource. When subresources are scraped for the `instances` resource, they will also be
scraped for these flavor-specific instance resources. The flavor-specific instance resources are in the `per_flavor`
category.

## `dns`: Designate v2

```yaml
services:
  - type: dns
```

The area for this service is `dns`.

| Resource | Unit |
| --- | --- |
| `recordsets` | countable |
| `zones` | countable |

When the `recordsets` quota is set, the backend quota for records is set to 20 times that value, to fit into the `records_per_recordset` quota (which is set to 20 by default in Designate). The record quota cannot be controlled explicitly in Limes.

## `network`: Neutron v1

```yaml
services:
  - type: object-store
```

The area for this service is `network`. Resources are categorized into `networking` for SDN resources and `loadbalancing` for LBaaS resources.

| Category | Resource | Unit | Comment |
| --- | --- | --- | --- |
| `networking` | `floating_ips` | countable ||
|| `networks` | countable ||
|| `ports` | countable ||
|| `rbac_policies` | countable ||
|| `routers` | countable ||
|| `security_group_rules` | countable | See note about auto-approval below. |
|| `security_groups` | countable | See note about auto-approval below. |
|| `subnet_pools` | countable ||
|| `subnets` | countable ||
| `loadbalancing` | `healthmonitors` | countable ||
|| `l7policies` | countable ||
|| `listeners` | countable ||
|| `loadbalancers` | countable ||
|| `pools` | countable ||

When a new project is scraped for the first time, and usage for `security_groups` and `security_group_rules` is 1 and 4,
respectively, quota of the same size is approved automatically. This covers the `default` security group that is
automatically created in a new project.

## `object-store`: Swift v1

```yaml
services:
  - type: object-store
```

The area for this service is `storage`.

| Resource | Unit |
| --- | --- |
| `capacity` | Bytes |

## `sharev2`: Manila v2

```yaml
services:
  - type: sharev2
```

The area for this service is `storage`.

| Resource | Unit |
| --- | --- |
| `shares` | countable |
| `share_capacity` | GiB |
| `share_networks` | countable |
| `share_snapshots` | countable |
| `snapshot_capacity` | GiB |

## `volumev2`: Cinder v2

```yaml
services:
  - type: volumev2
```

The area for this service is `storage`.

| Resource | Unit |
| --- | --- |
| `capacity` | GiB |
| `snapshots` | countable |
| `volumes` | countable |

Quotas per volume type cannot be controlled explicitly in Limes.

The `volumes` resource supports subresource scraping. Subresources bear the following attributes:

| Attribute | Type | Comment |
| --- | --- | --- |
| `id` | string | volume UUID |
| `name` | string | volume name |
| `status` | string | volume status [as reported by OpenStack Cinder](https://developer.openstack.org/api-ref/block-storage/v2/index.html#volumes-volumes) |
| `size` | integer value with unit | volume size |

# Available capacity plugins

Note that capacity for a resource only becomes visible when the corresponding service is enabled in the
`clusters.$id.services` list as well.

## `cinder`

```yaml
capacitors:
  - id: cinder
    cinder:
      volume_backend_name: "vmware"
```

| Resource | Method |
| --- | --- |
| `volumev2/capacity` | The sum over all pools reported by Cinder. |

The `cinder.volume_backend_name` parameter can be used to filter the back-end storage pools by volume name.

No estimates are made for the `snapshots` and `volumes` resources since capacity highly depends on
the concrete Cinder backend.

## `manila`

```yaml
capacitors:
- id: manila
  manila:
    share_networks: 250
    shares_per_pool: 1000
    snapshots_per_share: 5
    capacity_balance: 0.5
    capacity_overcommit: 1
```

| Resource | Method |
| --- | --- |
| `sharev2/share_networks` | Taken from identically-named configuration parameter. |
| `sharev2/shares` | Calculated as `shares_per_pool * count(pools) - share_networks`. |
| `sharev2/share_snapshots` | Calculated as `snapshots_per_share` times the above value. |
| `sharev2/share_capacity`<br>`sharev2/snapshot_capacity` | Calculated as `capacity_overcommit * sum(pool.capabilities.totalCapacityGB)`, then divided among those two resources according to the `capacity_balance` (see below). |

The capacity balance is defined as

```
snapshot_capacity = capacity_balance * share_capacity,
```

that is, there is `capacity_balance` as much snapshot capacity as there is share capacity. For example, `capacity_balance = 0.5` means that the capacity for snapshots is half as big as that for shares, meaning that shares get 2/3 of the total capacity and snapshots get the other 1/3.

## `manual`

```yaml
capacitors:
  - id: manual
    manual:
      network:
        floating_ips: 8192
        networks: 4096
```

The `manual` capacity plugin does not query any backend service for capacity data. It just reports the capacity data
that is provided in the configuration file in the `manual` key. Values are grouped by service, then by resource.

This is useful for capacities that cannot be queried automatically, but can be inferred from domain knowledge. Limes
also allows to configure such capacities via the API, but operators might prefer the `manual` capacity plugin because it
allows to track capacity values along with other configuration in a Git repository or similar.

## `nova`

```yaml
capacitors:
  - id: nova
    nova:
      vcpu_overcommit: 4
      hypervisor_type_pattern: '^(?:VMware|QEMU)'
      extra_specs:
        first: 'foo'
        second: 'bar'
```

| Resource | Method |
| --- | --- |
| `compute/cores` | The sum of the reported CPUs for all hypervisors, optionally multiplied by the `nova.vcpu_overcommit` parameter. This option is provided because the hypervisor statistics reported by Nova do not take overcommit into account. |
| `compute/instances` | Estimated as `10000 * count(availabilityZones)`, but never more than `sumLocalDisk / maxDisk`, where `sumLocalDisk` is the sum of the local disk size for all hypervisors, and `maxDisk` is the largest disk requirement of all flavors. |
| `compute/ram` | The sum of the reported RAM for all hypervisors. |

If the `nova.hypervisor_type_pattern` parameter is set, only those hypervisors are considered whose `hypervisor_type`
matches this regex. Note that this is distinct from the `hypervisor_type_rules` used by the `compute` quota plugin, and
uses the `hypervisor_type` reported by Nova instead.

The `nova.extra_specs` parameter can be used to control how flavors are enumerated. Only those flavors will be
considered which have all the extra specs noted in this map, with the same values as defined in the configuration file.
This is particularly useful to filter Ironic flavors, which usually have much larger root disk sizes.

## `prometheus`

```yaml
capacitors:
  - id: prometheus
    prometheus:
      api_url: https://prometheus.example.com
      queries:
        compute:
          cores:     sum(hypervisor_cores)
          ram:       sum(hypervisor_ram_gigabytes) * 1024
```

Like the `manual` capacity plugin, this plugin can provide capacity values for arbitrary resources. A [Prometheus][prom]
instance must be running at the URL given in `prometheus.api_url`. Each of the queries in `prometheus.queries` is
executed on this Prometheus instance, and the resulting value is reported as capacity for the resource named by the key
of this query. Queries are grouped by service, then by resource.

For example, the following configuration can be used with [swift-health-statsd][shs] to find the net capacity of a Swift cluster with 3 replicas:

```yaml
capacitors:
  - id: prometheus
    prometheus:
      api_url: https://prometheus.example.com
      queries:
        object-store:
          capacity: min(swift_cluster_storage_capacity_bytes < inf) / 3
```

## `sapcc-ironic`

```yaml
capacitors:
  - id: sapcc-ironic
```

This capacity plugin reports capacity for the special `compute/instances_<flavorname>` resources that exist on SAP
Converged Cloud ([see above](#compute-nova-v2)). For each such flavor, it counts the number of Ironic nodes whose RAM
size, disk size, number of cores, and capabilities match those in the flavor.

```yaml
capacitors:
  - id: sapcc-ironic
subcapacities:
  - compute: [ instances-baremetal ]
```

When the "compute/instances-baremetal" pseudo-resource is set up for subcapacity scraping (as shown above),
subcapacities will be scraped for all resources reported by this plugin. Subcapacities correspond to Ironic nodes and
bear the following attributes:

| Attribute | Type | Comment |
| --- | --- | --- |
| `id` | string | node UUID |
| `name` | string | node name |
| `ram` | integer value with unit | amount of memory |
| `cores` | integer | number of CPU cores |
| `disk` | integer value with unit | root disk size |
| `serial` | string | hardware serial number for node |

[yaml]:   http://yaml.org/
[pq-uri]: https://www.postgresql.org/docs/9.6/static/libpq-connect.html#LIBPQ-CONNSTRING
[policy]: https://docs.openstack.org/security-guide/identity/policies.html
[ex-pol]: ../example-policy.json
[prom]:   https://prometheus.io
[shs]:    https://github.com/sapcc/swift-health-statsd
