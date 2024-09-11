#!/usr/bin/env python3

import os
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:3030/v1")

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
