# export OTEL_SERVICE_NAME=heart
export OTEL_RESOURCE_ATTRIBUTES=deployment.environment=local,service.version=0.1.0,service.instance.id=$(HOSTNAME)
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4318/v1/traces
# export OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/protobuf
# export OTEL_TRACES_EXPORTER=otlp

go run .
