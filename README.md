# LFS Proxy

This service is a pull-through S3 cache for [Git LFS](https://git-lfs.com/) that caches Upstream LFS objects on S3 to reduce bandwidth costs on Hosted LFS, such as GitHub LFS.

These cached objects are served directly from S3 using Presigned requests for maximum performance.

Requests are cached in-memory using [bigcache](https://github.com/allegro/bigcache) to reduce the amount of HTTP calls to S3

## Installing the LFS Proxy

We are providing a [Helm Chart](https://github.com/vela-games/lfsproxy/tree/main/install/helm/lfsproxy) and an example [terrafom module](https://github.com/vela-games/lfsproxy/tree/main/install/terraform/lfsproxy) for deploying the service 

You'll need to make sure the service account deployed by the chart uses an IAM Role with access to S3 if you are running it on Kubernetes.

We currently don't have any public repositories for the Docker Image or the Helm chart, but is something we are looking into.

## Configurations

All configurations are loaded from environment variables using [envconfig](https://github.com/kelseyhightower/envconfig).

| Configuration Name             | Environment Variable                 | Default Value                                    | Description                                                                                       |
|--------------------------------|--------------------------------------|--------------------------------------------------|---------------------------------------------------------------------------------------------------|
| DebugMode                      | APP_DEBUG_MODE                       | false                                            | Enable gin-gonic debug mode                                                                       |
| UpstreamBaseURL                | APP_UPSTREAM_BASE_URL                |                                                  | The LFS Git Repository base url (Example: https://github.com/vela-games/example.git/info/lfs/)    |
| S3Bucket                       | APP_S3_BUCKET                        |                                                  | S3 Bucket Name                                                                                    |
| S3UseAccelerate                | APP_S3_USE_ACCELERATE                | false                                            | If S3 Accelerate URLs should be returned                                                          |
| S3PresignEnabled               | APP_S3_PRESIGN_ENABLED               | true                                             | If S3 Presign URLs should be used                                                                 |
| S3PresignExpiration            | APP_S3_PRESIGN_EXPIRATION            | 24h                                              | Presign Expiration                                                                                |
| CacheEviction                  | APP_CACHE_EVICTION                   | 23h                                              | When to evict cached requests from memory                                                         |
| EnablePrometheusExporter       | APP_ENABLE_PROMETHEUS_EXPORTER       | false                                            | Enable Prometheus exporter endpoint (/metrics)                                                    |
