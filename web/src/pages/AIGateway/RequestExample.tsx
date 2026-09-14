import {Field, Select} from '@/components/ui';
import LLMProtocol from '@/components/icons/LLMProtocol';
import {useI18n} from '@/i18n';
import {useState} from 'react';

export default function RequestExample({base, model, protocols}: {base:string; model:string; protocols?:string[]}) {
  const {tr}=useI18n();
  const [selected,setSelected]=useState('openai-compatible');
  const native=selected==='anthropic' && protocols?.includes('anthropic');
  const protocol=native?'anthropic':'openai-compatible';
  const payload={model, messages:[{role:'user',content:'Hello'}], ...(native?{max_tokens:1024}:{}), stream:true};
  // Quote JSON safely even if an administrator uses punctuation in an alias.
  const body=JSON.stringify(payload).replace(/'/g,"'\\''");
  const headers=native ? '  -H "x-api-key: $LIAISON_API_KEY" \\\n  -H \'anthropic-version: 2023-06-01\'' : '  -H "Authorization: Bearer $LIAISON_API_KEY"';
  return <>
    {protocols?.includes('anthropic') && <Field label={<span className="liaison-inline-name">{tr('调用协议','Request protocol')}<LLMProtocol protocol={protocol}/></span>}>
      <Select aria-label={tr('调用协议','Request protocol')} value={protocol} onChange={e=>setSelected(e.target.value)}>
        <option value="openai-compatible">OpenAI</option><option value="anthropic">Anthropic</option>
      </Select>
    </Field>}
    <pre className="ai-api-example">{`curl '${window.location.origin}${base}/v1/${native?'messages':'chat/completions'}' \\\n${headers} \\\n  -H 'Content-Type: application/json' \\\n  -d '${body}'`}</pre>
  </>;
}
