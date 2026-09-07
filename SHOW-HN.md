# Show HN: TraceShrink – OTel processor that keeps expensive LLM traces, drops cheap ones (98% storage savings)

I built TraceShrink, an OpenTelemetry Collector processor for AI/LLM observability pipelines.

**The problem:** If you're running LLM-powered services, your observability backend is drowning in traces. Most of them are $0.001 health checks and cache hits. The expensive ones — a $2.50 GPT-4o call that timed out, a chain-of-thought that burned 50K tokens — get randomly dropped by tail sampling alongside everything else.

Random 1% sampling doesn't know what's expensive. Head-based sampling misses errors that happen late. Langfuse/Helicone are SaaS — another vendor, another bill.

**What TraceShrink does:** It sits in your existing OTel Collector pipeline and evaluates each trace:
- `cost >= $0.10`? → Keep
- Has an error span? → Keep  
- Duration > 5s? → Keep
- Otherwise → Drop

One YAML block. No OTTL expressions. No SaaS.

**Real numbers (not projections):**
- 1M spans → 20K spans (98% storage reduction)
- 70% of total $ cost captured in 2% of spans
- 1.7M spans/sec throughput, 2ns per decision, 0 allocations
- Built-in pricing database (2,700+ LLM models via LiteLLM) auto-calculates cost from token counts

**How it fits your pipeline:**
```yaml
processors:
  costprocessor: {}    # annotates spans with gen_ai.usage.cost_usd
  traceshrink:
    cost_threshold: 0.10
    keep_errors: true
```

The `costprocessor` reads `gen_ai.request.model` + `gen_ai.usage.input_tokens` / `output_tokens` from your spans, looks up pricing (GPT-4o, Claude, Gemini, etc.), and writes `gen_ai.usage.cost_usd`. Then `traceshrink` samples based on that cost.

I built this because the existing tail sampling processor has no concept of "this trace cost money." You can approximate it with OTTL conditions, but you can't aggregate cost across spans in a trace.

Apache-2.0, Go, zero dependencies beyond the OTel Collector SDK.

GitHub: https://github.com/TimurRakhmatullin86/traceshrink
