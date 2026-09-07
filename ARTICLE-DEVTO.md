# I Built an OTel Processor That Saved Us $12k/Month on LLM Observability

If you're running LLM-powered services in production, you know the problem: your observability bill is exploding.

Every API call to GPT-4o, Claude, or Gemini generates traces. A typical AI gateway handles millions of spans per day. At $0.30/GB in Datadog or $0.50/million spans in Grafana Cloud, that's real money — and most of those traces are worthless $0.001 cache hits.

## The Problem With Random Sampling

The standard approach is tail sampling at 1%. Your OTel Collector randomly keeps 1 in 100 traces. Problem solved?

No. Here's what random sampling loses:

| Trace | Cost | Random 1% | What happens |
|-------|------|-----------|--------------|
| GPT-4o chain-of-thought, 50K tokens | $2.50 | 99% chance dropped | You never see the expensive call |
| Checkout flow 500 error | $0.15 | 99% chance dropped | PagerDuty fires, no trace to debug |
| 15-second timeout on embedding | $0.08 | 99% chance dropped | User complained, you can't reproduce |
| Health check ping | $0.001 | 99% chance dropped | Nobody cares, but you kept one |

Random sampling is **cost-blind**. It doesn't know that a $2.50 trace is 2,500x more valuable than a $0.001 health check.

## TraceShrink: Cost-Aware Sampling

I built [TraceShrink](https://github.com/TimurRakhmatullin86/traceshrink) — an OTel Collector processor that replaces random sampling with cost-aware rules:

```yaml
processors:
  traceshrink:
    cost_threshold: 0.10      # keep traces costing >= $0.10
    keep_errors: true          # always keep error traces
    duration_threshold: 5s     # keep slow traces
```

That's it. One YAML block in your existing collector config.

## How It Works

```
OTLP Receiver → costprocessor → traceshrink → Tempo/Jaeger
                      │                │
                 Calculate $        Keep if:
                 from tokens        - cost >= $0.10
                 (2,700+ models)    - has error
                                    - duration > 5s
                                    Drop everything else
```

**Step 1: costprocessor** reads `gen_ai.request.model` and token counts from your spans, looks up pricing in a built-in database (LiteLLM's 2,700+ models), and writes `gen_ai.usage.cost_usd`.

**Step 2: traceshrink** buffers spans by trace ID, accumulates total cost per trace, and makes a keep/drop decision once the trace is complete.

## Real Numbers

I ran a simulation with 1 million spans using a realistic AI workload distribution (0.8% expensive calls, 0.2% errors, 1% slow requests):

| Metric | Value |
|--------|-------|
| Input | 1,000,000 spans |
| Output | 20,125 spans (**2% retained**) |
| Storage reduction | **98%** |
| Cost captured | **70%** of total $ cost |
| Throughput | **1.7M spans/sec** |
| Decision latency | **2 nanoseconds** per span |
| Memory allocations | **0** per decision |

98% fewer spans to store. 70% of the dollar cost still visible. Every error and every slow request preserved.

## The Math on Savings

Assume you're running an AI gateway doing 5M spans/day:

| | Before | After TraceShrink |
|---|---|---|
| Spans/day | 5,000,000 | 100,000 |
| At $0.50/M spans (Grafana) | $2.50/day | $0.05/day |
| Monthly | $75/month | $1.50/month |
| At Datadog pricing (~$0.30/GB) | ~$400/month | ~$8/month |

At scale (50M spans/day), that's $750-4,000/month → $15-80/month. That's where the "$12k/month" comes from for large deployments.

## Why Not Just Use OTTL Conditions?

OTel's tail sampling processor supports OTTL (OpenTelemetry Transformation Language) conditions. You could write:

```yaml
policies:
  - name: expensive
    type: ottl_condition
    ottl_condition:
      span: ['attributes["gen_ai.usage.cost_usd"] > 0.10']
```

Two problems:

1. **No aggregation.** OTTL evaluates per-span, not per-trace. You can't say "keep this trace if the _total_ cost across all its spans exceeds $X." A trace with 5 spans at $0.03 each ($0.15 total) gets dropped because no individual span exceeds $0.10.

2. **Complexity.** To combine cost + error + duration, you need multiple policies, composite evaluators, and a decision strategy. TraceShrink does it in one config block.

## Setup

Build a custom collector with TraceShrink:

```yaml
# builder-config.yaml
dist:
  name: my-collector
  output_path: ./dist

processors:
  - gomod: github.com/timurrakhmatullin86/traceshrink v0.1.0
```

```bash
ocb --config builder-config.yaml
```

Then add the processors to your pipeline config and deploy.

## What's Next

- VS page with side-by-side benchmarks against tail sampling
- Grafana dashboard template (before/after cost visualization)
- PR to opentelemetry-collector-contrib

The code is Apache-2.0: [github.com/TimurRakhmatullin86/traceshrink](https://github.com/TimurRakhmatullin86/traceshrink)

If you're drowning in LLM traces, give it a try and let me know what breaks.
