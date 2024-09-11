#!/usr/bin/env python3

import os
import asyncio
from openai import AsyncOpenAI

client = AsyncOpenAI(base_url="http://127.0.0.1:3030/v1")

async def main() -> None:
    chat_completion = await client.chat.completions.create(
        messages=[
            {
                "role": "user",
                "content": "Say this is a test",
            }
        ],
        model="gpt-4o-mini",
    )
    print(chat_completion.choices[0].message.content)

asyncio.run(main())
