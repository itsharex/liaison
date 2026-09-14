import {ProviderIcon} from '@/components/AgentWorkspace/ProviderIcon';
import {useI18n} from '@/i18n';
import ollamaLogo from './ollama.svg';

export default function LLMProtocol({protocol}:{protocol?:string}) {
  const {tr}=useI18n();
  if(protocol==='ollama')return <span className="liaison-inline-name" title="Ollama native API"><span aria-hidden style={{display:'inline-block',width:18,height:18,flexShrink:0,background:'currentColor',mask:`url(${ollamaLogo}) center / contain no-repeat`,WebkitMask:`url(${ollamaLogo}) center / contain no-repeat`}}/><span>Ollama</span></span>;
  const provider=protocol==='openai-compatible'?'openai':protocol==='anthropic'?'anthropic':undefined;
  const label=provider==='openai'?'OpenAI':provider==='anthropic'?'Anthropic':tr('未配置','Not configured');
  return <span className="liaison-inline-name" title={provider==='openai'?'OpenAI-compatible':provider==='anthropic'?'Anthropic Messages':undefined}>
    {provider&&<ProviderIcon provider={provider} size={18}/>}
    <span>{label}</span>
  </span>;
}
