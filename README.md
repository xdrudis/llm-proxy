# llm-proxy

*💰 Save 50% on OpenAI API costs with a one-line change.*

OpenAI offers a 50% discount through their [batch API](https://platform.openai.com/docs/guides/batch/overview)
if you can tolerate a higher latency and are willing
to make significant changes to your code.

With llm-proxy, you can start using OpenAI’s batch API by
configuring the OpenAI client to use the proxy as the base URL.
No other code changes required.

Your application sends a request and waits for a response just like before, except with increased latency.

## How It Works

1. The proxy receives individual API requests from the application.
2. It groups them into batches based on configurable criteria (time window, batch size, etc.).
3. Batched requests are sent to OpenAI's batch API endpoint.
4. The proxy waits until the batch finishes.

<p align="center">
	<img src='/imgs/diagram.png'>
</p>

## Getting Started

You can run it via Docker or directly in a Go environment.

Docker (a lightweight 26MiB image):
```
docker build -t llm-proxy .
docker run -d --restart unless-stopped -p 3030:3030 llm-proxy
```

Directly using Go:
```
go run .
```

## Examples
See the [examples folder](/tree/main/examples) for more details and usage examples.

#### Python
```python
import os
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:3030/v1") # only change needed

completion = client.chat.completions.create(
    messages=[
        {
            "role": "user",
            "content": "Say this is a test",
        }
    ],
    model="gpt-4o-mini",
)

print(completion.choices[0].message.content)
```

#### Node
```typescript
import OpenAI from "openai";

const openai = new OpenAI({
	baseURL: 'http://127.0.0.1:3030/v1' // only change needed
});

async function main() {
  const completion = await openai.chat.completions.create({
    model: 'gpt-4o-mini',
    messages: [{ role: 'user', content: 'Say this is a test' }],
  });
  console.log(completion.choices[0]?.message?.content);
}

main();
```

#### Curl
```sh
curl http://127.0.0.1:3030/v1/chat/completions \
 -H "Content-Type: application/json" \
 -H "Authorization: Bearer $OPENAI_API_KEY" \
 -d '{
     "model": "gpt-4o-mini",
     "messages": [{"role": "user", "content": "Say this is a test!"}],
     "temperature": 0.7
   }'
```

#### Go
```go
package main

import (
	"context"
	"fmt"
	openai "github.com/sashabaranov/go-openai"
	"os"
)

func main() {
	config := openai.DefaultConfig(os.Getenv("OPENAI_API_KEY"))
	config.BaseURL = "http://127.0.0.1:3030/v1" // only change needed
	client := openai.NewClientWithConfig(config)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Hello!",
				},
			},
		},
	)
	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return
	}
	fmt.Println(resp.Choices[0].Message.Content)
}
```

## Supported endpoints
Both APIs that support batch ([`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat) and [`/v1/embeddings`](https://platform.openai.com/docs/api-reference/embeddings)) are supported.

Any other endpoint will be relayed to OpenAI as-is.

## Monitoring
Simple real-time statistics are accessible through the `http://127.0.0.1:3030/stats` endpoint. This provides insights into request counts, batch efficiency, and latency metrics.
Monitor the `/stats` endpoint to ensure the proxy is performing as expected in your environment.

Sample output:
```json
{
  "requests": {
    "total": 2998,
    "successful": 2997,
    "failed": 0,
    "synthesized_error_responses": 999,
    "avg_time_ms": 153959.67467467466,
    "p50_time_ms": 203733,
    "p95_time_ms": 250896,
    "p99_time_ms": 251073
  },
  "batches": {
    "total": 3,
    "successful": 3,
    "failed": 0,
    "avg_time_ms": 152846.33333333334,
    "p50_time_ms": 104655.5,
    "p95_time_ms": 226016,
    "p99_time_ms": 226016
  }
}
```

## Limitations
- Not suitable for applications requiring real-time responses (e.g. chatbot)
- Streaming APIs are not supported, as they don't support batch mode.

## A note about latency
OpenAI's commitment for this API is 24-hour turnaround time.
In practice, I've observed good latencies during nights and weekends (a few seconds).
However, latency can increase significantly during weekdays, sometimes reaching up to an hour.

## Future work
- Configurable grace period. If a batch doesn't complete within the allotted time, 
the batch will be canceled, partial results will be returned,
and the remaining requests will be sent via the synchronous API.
- Add support also for Gemini, as it also supports batch.
- Enable usage tracking (counting tokens, estimating cost).
