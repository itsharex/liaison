import type {ReactNode} from 'react';
import {useI18n} from '@/i18n';
import './AccessContext.less';

export default function AccessContext({name,protocol,target,children}:{name:string;protocol:ReactNode;target?:string;children?:ReactNode}){
 const {tr}=useI18n();
 return <div className="liaison-access-context" aria-label={tr('访问信息','Access information')}>
  <div><span>{tr('名称','Name')}</span><span>{name}</span>{children}</div>
  <div><span>{tr('协议','Protocol')}</span><span>{protocol}</span></div>
  {target&&<div><span>{tr('目标','Target')}</span><code>{target}</code></div>}
 </div>;
}
