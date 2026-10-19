# ngsi-context-broker

A generic context broker for federation of our context sources.

Requires Go 1.27 or newer. The container build uses `golang:1.27`.

## Snapshot reconciler

`snapshot-reconciler` is a standalone, one-shot job for repairing Orion-LD
snapshots after measurements arrive out of order. It reads snapshots through
Orion-LD's API and history through Mintaka's API; it does not access either
database directly. Job-specific implementation and tests are isolated under
`cmd/snapshot-reconciler` and do not change the running context broker.

The binary is included at `/opt/diwise/snapshot-reconciler` in the service image.
Locally, run it with:

```sh
go run ./cmd/snapshot-reconciler -config deployments/configs/context-broker.yaml
go run ./cmd/snapshot-reconciler -config deployments/configs/context-broker.yaml -tenant default -type WeatherObserved -apply
go run ./cmd/snapshot-reconciler -config deployments/configs/context-broker.yaml -id urn:ngsi-ld:WeatherObserved:example -apply
```

Dry-run is the default. Only `-apply` enables writes. The job uses the existing
configuration's `endpoint`, `temporal.endpoint`, tenant IDs, registered types and
ID patterns. Only sources with `temporal.enabled: true` are selected. If the
temporal endpoint is omitted, the source endpoint is used. Configured tenant IDs
select context sources, just as in the running context broker; they are **not**
automatically sent as physical Orion/Mintaka tenant headers. By default the job
uses the tenantless database. For installations with a physical broker tenant,
set `-tenant <configured-id> -ngsild-tenant <physical-id>`; the latter is sent to
both APIs (`default` denotes the tenantless database). Sources must be accessible
from the job without an interactive login; URLs containing credentials are rejected.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `/opt/diwise/config/default.yaml` | Existing context-broker configuration |
| `-apply` | `false` | Apply repairs instead of dry-run |
| `-tenant` | all | Restrict to a configured tenant ID |
| `-ngsild-tenant` | empty | Explicit physical broker tenant; requires `-tenant` |
| `-type` | all | Restrict to a registered entity type |
| `-id` | all | Restrict to one registered entity ID |
| `-context` | Diwise default context URL | Same JSON-LD context used for both APIs |
| `-page-size` | `100` | Orion entities per page, between 1 and 1000 |
| `-request-timeout` | `30s` | Timeout per HTTP request |
| `-timeout` | `30m` | Deadline for the entire job |

### Repair semantics

Entities are scanned serially with `options=sysAttrs`. History is selected per
eligible attribute with `attrs=<attribute>&lastN=1&timeproperty=observedAt&options=sysAttrs`.
That response is **not** used as the repair payload: Mintaka also applies
`lastN=1` to subattributes and can silently omit metadata. The selected write is
fetched again without `lastN`, in a narrow `modifiedAt` time window, and matched
by `instanceId`. All returned subattributes are then preserved. A partial
response, missing/duplicate instance, changed core value or a result at Mintaka
0.7.0's known 1000-instance cap is skipped. Subattribute saturation is checked
explicitly because Mintaka does not indicate it in the HTTP status. This
completeness check targets Mintaka 0.7.0's fixed limit; revalidate that limit if
upgrading to a version with different temporal query limits.

An existing snapshot attribute is repaired only when history has a strictly
later `observedAt`. The historical value, original observation time, unit and supported nested
attributes are preserved. Temporal `instanceId`, `createdAt` and `modifiedAt`
are not copied into repair requests. Compound values remain intact.

The historical write's `modifiedAt` must also be strictly after both the
current entity's and attribute's `createdAt`. Candidates from a previous
lifecycle are skipped. Missing/invalid lifecycle
timestamps cause a skip. Both APIs expose these system timestamps with
millisecond precision: writes at the creation boundary are therefore ambiguous
and are skipped, including a latest observation originally written during
creation. If the globally latest observation belongs to an earlier lifecycle,
the attribute is skipped rather than searching for a different candidate.

Before writing, the job rereads the snapshot with system attributes. Attributes
or creation timestamps changed since the initial read are deferred to a later
run. It PATCHes only the remaining stale attributes and verifies them by reading
back. This is periodic reconciliation,
not an atomic guarantee against concurrent ingestion. Pagination is also not a
transactional snapshot: concurrent creation/deletion can defer entities to the
next run. Already corrected attributes cause no further writes on subsequent
runs.

Every actual repair is a normal Orion-LD write and can generate notifications
and an extra TRoE history instance with the original `observedAt`. These extra
instances are accepted and may affect count/aggregation queries. The job does
not invoke the context broker's own notification application.

The first version supports single `Property`, `GeoProperty` and `Relationship`
objects. It reports/skips missing or invalid observation times, datasetId
attributes, snapshot arrays, ambiguous temporal instances, unsupported nested
representations and conflicting contents at equal timestamps. Absent snapshot
attributes and deleted entities are not recreated. Mintaka's `lastN=1` does not
resolve conflicting historical values at the same timestamp; the job does not
attempt historical conflict resolution.

Logs contain IDs, attribute names and timestamps, not measurement payloads.
The final summary reports entities, stale attributes, verified corrections,
skips and errors. HTTP, decoding and verification errors produce a nonzero exit
status; skips do not. SIGINT/SIGTERM cancel in-flight work. There are no automatic
write retries; a later run rereads state before deciding whether a repair is
still needed.

### Verification

The real-backend integration test targets Orion-LD **1.12.0** with TRoE enabled
and Mintaka **0.7.0**. Use isolated test services (the test creates unique
entities in the tenantless database and a unique tenant; deleting an entity
does not purge its TRoE history):

```sh
RECONCILER_TEST_ORION_URL=http://localhost:1026 \
RECONCILER_TEST_MINTAKA_URL=http://localhost:1027 \
go test ./cmd/snapshot-reconciler -run TestBackendIntegration -count=1 -v
```

Without both URLs this integration test is explicitly skipped. HTTP tests run
without backend services and cover dry-run, multiple subattributes, partial or
saturated history, lifecycle boundaries, changed or deleted snapshots, repeated
execution, partial writes, API failures, cancellation and malformed responses.
The real-backend test additionally recreates entities and attributes with the
same ID to verify that their earlier lifecycle is not restored, and tests 1001
subattributes and an observation written at the creation boundary. HTTP tests
do not prove real TRoE or notification behavior.

## TRoE cleaner

`/opt/diwise/troe-cleaner` removes redundant, consecutive writes from the
Orion-LD TRoE database. Its purpose is to reduce history growth when clients
repeatedly sync unchanged objects or fields, not to make every historical row
globally unique. Locally it can be run with `go run ./cmd/troe-cleaner`.

The existing database configuration is unchanged:

| Environment variable | Default |
| --- | --- |
| `POSTGRES_HOST` | empty |
| `POSTGRES_USER` | empty |
| `POSTGRES_PASSWORD` | empty |
| `POSTGRES_PORT` | `5432` |
| `POSTGRES_DBNAME` | `diwise` |
| `POSTGRES_SSLMODE` | `disable` |

Set `POSTGRES_DBNAME` to the actual TRoE database, for example `orion` or
`orion_<physical-tenant>`. The cleaner processes that database only; it does
not enumerate other tenant databases or read the federation configuration.
It writes immediately, as before, and has no dry-run mode.

### Compaction rule

Rows are compared in **write-time order** (`ts`), separately for each entity,
attribute and dataset. An `Update` or `Replace` is removed only when its
stored content is identical to the preceding state. The comparison includes
the value and value type, `observedAt`, unit and the complete set of
subattributes. Generated instance IDs, write times, operation modes and
transport correlators are excluded from content equality. Different physical
representations are conservatively retained, rather than normalized by model.

For a description without observation time:

```text
A → A → A → B → B → A → A
```

becomes:

```text
A → B → A
```

The **first** row of each unchanged run remains, preserving when the state
started. A new observation time is new content even if the measurement value
has not changed. Entity creation/deletion boundaries separate histories, and
attribute `Create`, `Append` and `Delete` markers are retained. Unknown
operation types are not removed. Equal or otherwise ambiguous write-time
ordering is left untouched around the ambiguity. Histories without an entity
lifecycle marker are also left untouched. Some redundant rows can therefore
remain, intentionally.

Classification and deletion are one atomic SQL statement per entity, not one
DELETE round trip per row. A failure rolls back both parent and child
deletions. Child rows are deleted only with their redundant parent, using
entity, instance and dataset keys; shared references to a surviving parent
version are preserved. This does not sweep up older orphaned rows created by
previous cleaner versions. Entity rows and the Mongo snapshot are unchanged.
Concurrent/newly arrived rows can be handled on a later run. SIGINT/SIGTERM
cancel database work through PostgreSQL's cancellation protocol with a bounded
socket-deadline fallback.

The summary counts actually deleted attributes and subattributes. After a
nonempty cleanup, normal `VACUUM ANALYZE` runs for both tables. This makes
deleted space reusable; it is not `VACUUM FULL` and does not guarantee that
database files shrink on disk. Large production histories still depend on
appropriate existing indexes and PostgreSQL resources. In particular,
`subattributes_attributeid_index` on `(attrinstanceid, attrdatasetid)` is
important and is present in the verified Orion-LD 1.12.0 schema. The job does
not change the schema or create indexes.

### Cleaner verification

PostgreSQL regression tests create and drop isolated schemas:

```sh
TROE_CLEANER_TEST_DATABASE_URL='postgres://postgres:password@localhost:5432/postgres?sslmode=disable' \
go test ./cmd/troe-cleaner -count=1 -v
```

They cover sync runs, `A → B → A`, observation-time changes, datasets, units,
subattributes, lifecycle markers, shared instance keys, repeated/concurrent
execution, cancellation, rollback on child-deletion failure and a 5000-write
sync run. The fixture includes Orion's subattribute reference index. Without
the database URL these tests explicitly skip.

For the real Orion-LD **1.12.0** / Mintaka **0.7.0** test, also set
`TROE_CLEANER_TEST_ORION_URL` and `TROE_CLEANER_TEST_MINTAKA_URL`. The database
URL must point to that stack's PostgreSQL server with access to the generated
`orion_<tenant>` database. Use isolated services: the test creates a unique
tenant and Beach, checks repeated full-field and single-field patches, checks
the compacted history through Mintaka and deletes the entity afterward. TRoE
history remains until the test environment is discarded. It uses the expanded
description IRI because the two versions map the compact term differently.
