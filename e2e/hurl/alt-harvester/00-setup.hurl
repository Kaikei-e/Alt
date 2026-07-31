# alt-harvester readiness probe — must run first. Retries /health on the
# operator listener until the compose healthcheck settles.
#
# :9110 is the only socket this process opens. alt-harvester is the
# scheduled-job third of the alt-backend split: it owns what
# job.RegisterAllJobs wired into the monolith and serves no API at all. The
# operator listener exists so compose has something to probe and Prometheus
# has something to scrape — it is not a product surface, which is why the
# path is `/health` and not the versioned public `/v1/health` namespace.
GET {{ops_url}}/health
[Options]
retry: 60
retry-interval: 1000
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "alt-harvester"
