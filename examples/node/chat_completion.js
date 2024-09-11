// node ./chat_completion.js 
import OpenAI from "openai";

const openai = new OpenAI({
	baseURL: 'http://127.0.0.1:3030/v1'
});

async function main() {
  const completion = await openai.chat.completions.create({
    model: 'gpt-4o-mini',
    messages: [{ role: 'user', content: 'Say this is a test' }],
  });
  console.log(completion.choices[0]?.message?.content);
}

main();
