# TraceShrink

[![CI](https://github.com/TimurRakhmatullin86/traceshrink/actions/workflows/ci.yml/badge.svg)](https://github.com/TimurRakhmatullin86/traceshrink/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/timurrakhmatullin86/traceshrink.svg)](https://pkg.go.dev/github.com/timurrakhmatullin86/traceshrink)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**Cost-aware trace sampling for OpenTelemetry Collector.**

Random tail-sampling drops 99% of traces — including the $2 LLM calls and the errors that page you at 3 AM. TraceShrink keeps only what matters: expensive spans, errors, and slow requests.

```
1,000,000 spans → 20,125 spans (2% retained)
98% storage reduction. 70% of total cost captured. 0% lost incidents.
1.7M spans/sec throughput. 575ns per span.
```

## How It Works

```
┌──────────┐    ┌─────────────────────────────────────┐    ┌──────────┐
│   OTLP   │───▶│         TraceShrink Processor        │───▶│  Tempo / │
│ Receiver │    │                                       │    │  Jaeger  │
└──────────┘    │  ┌─────────┐   ┌──────────────────┐  │    └──────────┘
                │  │ Buffer   │──▶│ Evaluate per trace│  │
                │  │ by trace │   │                    │  │
                │  │ ID       │   │ cost >= $0.10? ──▶ KEEP
                │  │          │   │ has error?     ──▶ KEEP
                │  │          │   │ duration > 5s? ──▶ KEEP
                │  │          │   │ otherwise      ──▶ DROP
                │  └─────────┘   └──────────────────┘  │
                └─────────────────────────────────────┘
```

## Quick Start

Add TraceShrink to your OTel Collector config:

```yaml
processors:
  traceshrink:
    cost_threshold: 0.10      # keep traces with cost >= $0.10
    keep_errors: true          # always keep error traces
    duration_threshold: 5s     # keep traces slower than 5s

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [traceshrink]
      exporters: [otlp]
```

## How It Works

TraceShrink is an OTel Collector processor that sits in your trace pipeline. Instead of random sampling (which blindly drops expensive traces) or head-based sampling (which misses late errors), TraceShrink evaluates each trace against cost, error, and duration rules:

| What | Random 1% | Head 1% | TraceShrink |
|------|-----------|---------|-------------|
| $2.50 LLM call | 99% dropped | 99% dropped | **Kept** |
| 500 error on checkout | 99% dropped | Kept if early | **Kept** |
| 15s timeout | 99% dropped | 99% dropped | **Kept** |
| $0.001 health check | 99% dropped | Kept if early | **Dropped** |

## Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cost_threshold` | float | `0.10` | Keep traces where any span cost >= this (USD). 0 = disabled. |
| `cost_attribute` | string | `"llm.cost"` | Span attribute name holding the cost value. |
| `keep_errors` | bool | `true` | Keep all traces containing an error span. |
| `duration_threshold` | duration | `5s` | Keep traces exceeding this duration. 0 = disabled. |
| `keep_attributes` | []string | `[]` | Keep traces with any of these attributes present. |
| `decision_wait` | duration | `30s` | Time to wait for a complete trace before deciding. |
| `num_traces` | uint | `100000` | Max traces held in memory pending a decision. |

## Performance

Benchmarked on Apple M3 Pro (real measurements, not targets):

| Metric | Value |
|--------|-------|
| Decision evaluation | **2 ns/span, 0 allocs** |
| 1K span batch | 0.36 ms |
| 10K span batch | 4.3 ms |
| Per-span overhead | ~575 ns |
| Throughput | **1.7M spans/sec** |

### 1M Span Simulation

With realistic AI/LLM workload distribution (0.8% expensive, 0.2% errors, 1% slow):

| Metric | Value |
|--------|-------|
| Input | 1,000,000 spans (200K traces) |
| Output | 20,125 spans (**2.0% retained**) |
| Storage reduction | **98%** |
| Cost captured | **70%** of total $ cost |
| Ingestion time | 575ms for 1M spans |

## Building a Custom Collector

Use the [OpenTelemetry Collector Builder (ocb)](https://opentelemetry.io/docs/collector/extend/ocb/) to include TraceShrink in your collector:

```yaml
# builder-config.yaml
dist:
  name: my-collector
  output_path: ./dist

processors:
  - gomod: github.com/timurrakhmatullin86/traceshrink v0.1.0
```

Then build:

```bash
ocb --config builder-config.yaml
```

## vs Tail Sampling

OTel's built-in `tailsampling` processor supports policy-based decisions, but it doesn't understand cost. You can write OTTL conditions, but there's no aggregation — you can't say "keep this trace if the total cost across all spans exceeds $X."

TraceShrink is purpose-built for the AI/LLM observability use case where every API call has a dollar cost attached. One YAML block, no OTTL expressions, no policy chains.

## vs Langfuse / Helicone / OpenLIT

These are SaaS platforms — another vendor, another bill, another data egress point. TraceShrink is a processor in your existing OTel Collector. Zero additional vendors. Your data stays in your pipeline.

## License

Apache-2.0
