import {useEffect,useState,useRef} from 'react';
import AccessContext from '@/components/AccessContext';
import {accessSource,useAccessBack} from '@/hooks/useAccessBack';
import {useNavigate,useParams,useSearchParams} from 'react-router-dom';
import {request} from '@/api/client';
import {Button,Notice,Modal,Field,Input} from '@/components/ui';
import {useI18n} from '@/i18n';
import Files,{type FileSession} from '@/pages/WebSSH/Files';
import {getWebSSHTarget} from '@/services/api';
import {useFeature} from '@/store/permissions';
import './index.less';

export default function WebSFTP(){const {proxyId}=useParams();return <FileAccess key={proxyId}/>;}
function FileAccess(){
  const id=Number(useParams().proxyId),navigate=useNavigate(),[search]=useSearchParams(),{tr}=useI18n();
  const allowed=useFeature('webssh.files.read');
  const [session,setSession]=useState<FileSession>(),[error,setError]=useState(false),[retry,setRetry]=useState(0);
  const [username,setUsername]=useState('');
  const [context,setContext]=useState<{name:string;target:string}>();
  const [prompt,setPrompt]=useState(false),[password,setPassword]=useState(''),[busy,setBusy]=useState(false);
  const mounted=useRef(true),[saved,setSaved]=useState(false);
  useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
  const back=accessSource(search.toString(),'/proxy?access_type=websftp');
  const goBack=useAccessBack(back);
  useEffect(()=>{
    if(!allowed)return;
    let alive=true,opened:string|undefined;setSession(undefined);setError(false);
    const release=(token:string)=>{void request(`/api/v1/webssh/files/sessions/${encodeURIComponent(token)}`,{method:'DELETE',preserveLoginOnUnauthorized:true}).catch(()=>{/* Session also expires server-side. */});};
    (async()=>{
      const target=await getWebSSHTarget(id);if(!alive)return;
      if(target.code!==200||target.data?.access_protocol!=='websftp')throw Error('target');
      setContext({name:target.data.proxy_name||'WebSFTP',target:`${target.data.target_host}:${target.data.target_port}`});
      const c=target.data.credentials?.[0];
      if(!c){navigate(`/proxy?access_type=websftp&configure=${id}`,{replace:true});return;}
      setUsername(c.username||'');
      setSaved(!!c.saved);
      if(!c.saved){setPrompt(true);return;}
      if(target.data.effective_status!=='active')throw Error('unavailable');
      const r=await request<API.Response<FileSession>>(`/api/v1/webssh/proxies/${id}/files/sessions`,{method:'POST',data:{username:c.username,use_saved_credential:true}});
      if(r.code!==200||!r.data?.id)throw Error('session');
      if(!alive){release(r.data.id);return;}opened=r.data.id;setSession(r.data);
    })().catch(()=>{if(alive)setError(true);});
    return()=>{alive=false;if(opened)release(opened);};
  },[id,allowed,retry,navigate]);
  const connectTemporary=async()=>{if(busy)return;setBusy(true);setError(false);try{
    const r=await request<API.Response<FileSession>>(`/api/v1/webssh/proxies/${id}/files/sessions`,{method:'POST',data:{username,password,use_saved_credential:false,save_credential:false}});
    if(r.code!==200||!r.data?.id)throw Error('session');
    if(!mounted.current){void request(`/api/v1/webssh/files/sessions/${encodeURIComponent(r.data.id)}`,{method:'DELETE',preserveLoginOnUnauthorized:true}).catch(()=>{});return;}
    setSession(r.data);setPassword('');setPrompt(false);
  }catch{if(mounted.current)setError(true);}finally{if(mounted.current)setBusy(false);}};
  if(!allowed)return <Notice tone="danger">{tr('没有文件浏览权限。','File browsing is not permitted.')}</Notice>;
  if(prompt)return <Modal open title={tr('连接验证','Connection authentication')} onClose={()=>{if(!busy){setPassword('');navigate(back);}}} closeOnMask={!busy} footer={<Button variant="primary" disabled={busy||!password} onClick={()=>void connectTemporary()}>{busy?tr('连接中…','Connecting…'):tr('连接','Connect')}</Button>}>
    <div className="liaison-form"><Field label={tr('用户名','Username')}><Input value={username} readOnly/></Field>
    <Field label={tr('密码','Password')} hint={tr('仅用于本次连接，不保存密码。','Used for this connection only. The password is not saved.')}><Input autoFocus type="password" autoComplete="off" value={password} onChange={e=>setPassword(e.target.value)} onKeyDown={e=>{if(e.key==='Enter'&&password)void connectTemporary();}}/></Field>
    {error&&<Notice tone="danger">{tr('连接失败，请检查密码和服务状态。','Connection failed. Check the password and service status.')}</Notice>}</div>
  </Modal>;
  if(error)return <div className="liaison-page-stack"><Notice tone="danger">{tr('无法连接文件服务，请检查访问配置和目标服务状态。','Unable to connect to the file service. Check access configuration and the target service.')}</Notice><div className="liaison-table-actions"><Button onClick={()=>setRetry(n=>n+1)}>{tr('重试','Retry')}</Button><Button onClick={()=>navigate(`/proxy?access_type=websftp&configure=${id}`)}>{tr('编辑访问','Edit access')}</Button><Button onClick={()=>navigate(back)}>{tr('返回访问','Back to access')}</Button></div></div>;
  if(!session)return <div role="status">{tr('正在连接文件服务…','Connecting to file service…')}</div>;
  return <div className="websftp-workspace"><AccessContext name={context?.name||'WebSFTP'} protocol="SFTP" target={context?.target}/><div className="websftp-browser"><Files proxyId={id} username={username} saved={saved} standalone initialSession={session} onClose={goBack}/></div></div>;
}
