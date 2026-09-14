import {useEffect, useRef, useState} from 'react';
import {useNavigate, useParams, useSearchParams} from 'react-router-dom';
import {Folder, File, Archive, RefreshCw, ArrowUp} from 'lucide-react';
import {Button, Field, Input, Notice} from '@/components/ui';
import AccessContext from '@/components/AccessContext';
import {request} from '@/api/client';
import {getWebDataTarget, createWebDataSession, deleteWebDataSession} from '@/services/api';
import {useI18n} from '@/i18n';
import './index.less';

type Entry = {kind:'bucket'|'prefix'|'object';key:string;size?:number;modified_at?:string;etag?:string};
type Page = {rows?:Entry[];next_token?:string;error?:string};

export default function WebS3() {
  const {proxyId,credentialId}=useParams();
  return <StorageAccess key={`${proxyId}/${credentialId}`} />;
}

function StorageAccess() {
  const {proxyId,credentialId}=useParams(), [search]=useSearchParams(), navigate=useNavigate(), {tr}=useI18n();
  const [target,setTarget]=useState<API.WebDataTarget>();
  const [credential,setCredential]=useState<API.WebDataCredential>();
  const [token,setToken]=useState(''),[secret,setSecret]=useState(''),[error,setError]=useState(false),[busy,setBusy]=useState(true),[retry,setRetry]=useState(0);
  const opened=useRef(''),alive=useRef(true),generation=useRef(0);
  useEffect(()=>{
    alive.current=true;generation.current++; let current=true;
    setError(false);setBusy(true);
    (async()=>{
      const result=await getWebDataTarget(Number(proxyId));
      if(!current)return;
      const data=result.data,c=data?.credentials?.find(item=>item.id===Number(credentialId));
      if(result.code!==200||data?.protocol!=='s3'||!c)throw Error('target');
      setTarget(data);setCredential(c);
      if(c.saved)await connect(c,'');
      else setBusy(false);
    })().catch(()=>{if(current){setError(true);setBusy(false);}});
    return()=>{current=false;alive.current=false;generation.current++;if(opened.current){void deleteWebDataSession(opened.current).catch(()=>{});opened.current='';}};
  },[proxyId,credentialId,retry]);
  async function connect(c:API.WebDataCredential,password:string) {
    const run=generation.current;
    setBusy(true);setError(false);
    try {
      const result=await createWebDataSession(Number(proxyId),{
        ...(c.saved?{credential_id:c.id}:{username:c.username,database:c.database,schema:c.schema,tls_mode:c.tls_mode}),
        protocol:'s3',password,
      });
      if(result.code!==200||!result.data?.token)throw Error('session');
      if(!alive.current||run!==generation.current){void deleteWebDataSession(result.data.token).catch(()=>{});return;}
      opened.current=result.data.token;setToken(result.data.token);setSecret('');
      navigate(`/webs3/${proxyId}/connections/${credentialId}/sessions/${encodeURIComponent(result.data.token)}${search.size?`?${search}`:''}`,{replace:true});
    }catch{if(alive.current&&run===generation.current)setError(true);}finally{if(alive.current&&run===generation.current)setBusy(false);}
  }
  return <div className="webs3-workspace">
    <AccessContext name={target?.proxy_name||'WebS3'} protocol="WebS3" target={target?`${target.target_host}:${target.target_port}`:undefined}/>
    {error&&<Notice tone="danger">{tr('无法连接对象存储，请检查密钥、桶权限和目标服务。','Unable to connect. Check credentials, bucket permissions and the target service.')} <Button onClick={()=>{setToken('');setRetry(n=>n+1);}}>{tr('重试','Retry')}</Button></Notice>}
    {token?<ObjectBrowser token={token}/>:busy?<div role="status">{tr('正在连接对象存储…','Connecting to object storage…')}</div>:credential&&!credential.saved?<form className="webs3-auth" onSubmit={event=>{event.preventDefault();void connect(credential,secret);}}>
      <Field label="Access Key"><Input readOnly value={credential.username}/></Field>
      <Field label="Secret Key" hint={tr('仅用于本次会话，不保存。','Used for this session only; not saved.')}><Input autoFocus type="password" autoComplete="off" value={secret} onChange={event=>setSecret(event.target.value)}/></Field>
      <Button type="submit" variant="primary" disabled={!secret||busy}>{tr('连接','Connect')}</Button>
    </form>:null}
  </div>;
}

export function ObjectBrowser({token}:{token:string}) {
  const {tr}=useI18n();
  const [buckets,setBuckets]=useState<Entry[]>([]),[bucketNext,setBucketNext]=useState(''),[bucketBusy,setBucketBusy]=useState(false),[bucketError,setBucketError]=useState(false);
  const [bucket,setBucket]=useState(''),[prefix,setPrefix]=useState(''),[draft,setDraft]=useState(''),[parents,setParents]=useState<string[]>([]);
  const [rows,setRows]=useState<Entry[]>([]),[next,setNext]=useState(''),[tokens,setTokens]=useState<string[]>(['']),[page,setPage]=useState(0),[revision,setRevision]=useState(0);
  const [loading,setLoading]=useState(false),[failed,setFailed]=useState(false),[selected,setSelected]=useState<Entry>();
  const bucketRequest=useRef<AbortController>();
  const execute=async(command:object,signal:AbortSignal)=>{
    const result=await request<API.Response<Page>>(`/api/v1/webdata/sessions/${encodeURIComponent(token)}/execute`,{method:'POST',data:{statement:JSON.stringify(command)},signal,preserveLoginOnUnauthorized:true});
    if(result.code!==200||!result.data||result.data.error)throw Error('listing');
    return result.data;
  };
  async function loadBuckets(continuation='') {
    bucketRequest.current?.abort();const controller=new AbortController();bucketRequest.current=controller;
    setBucketBusy(true);setBucketError(false);
    try{const result=await execute({operation:'list_buckets',continuation_token:continuation},controller.signal);
      if(controller.signal.aborted)return;
      setBuckets(old=>continuation?[...old,...result.rows||[]]:result.rows||[]);setBucketNext(result.next_token||'');
    }catch{if(!controller.signal.aborted)setBucketError(true);}finally{if(!controller.signal.aborted)setBucketBusy(false);}
  }
  useEffect(()=>{void loadBuckets();return()=>bucketRequest.current?.abort();},[token]);
  useEffect(()=>{
    if(!bucket)return;
    const controller=new AbortController();setLoading(true);setFailed(false);setRows([]);setSelected(undefined);setNext('');
    execute({operation:'list_objects',bucket,prefix,continuation_token:tokens[page]||''},controller.signal).then(result=>{
      if(!controller.signal.aborted){setRows(result.rows||[]);setNext(result.next_token||'');}
    }).catch(()=>{if(!controller.signal.aborted)setFailed(true);}).finally(()=>{if(!controller.signal.aborted)setLoading(false);});
    return()=>controller.abort();
  },[token,bucket,prefix,page,revision]);
  function enter(name:string,path='',up=false){
    setBucket(name);setPrefix(path);setDraft(path);setPage(0);setTokens(['']);
    setParents(old=>name!==bucket||!path?[]:up?old.slice(0,-1):path===prefix?old:[...old,prefix]);
    setRevision(n=>n+1);
  }
  return <div className="webs3-browser">
    <aside className="webs3-sidebar">
      <div className="webs3-toolbar"><h2>{tr('存储桶','Buckets')}</h2><Button aria-label={tr('刷新存储桶','Refresh buckets')} loading={bucketBusy} onClick={()=>void loadBuckets()}><RefreshCw size={14}/></Button></div>
      {bucketError?<Notice tone="danger">{tr('列举失败，请刷新重试。','Listing failed. Refresh to retry.')}</Notice>:null}
      <nav aria-label={tr('存储桶','Buckets')}>{buckets.map(item=><Button key={item.key} variant="ghost" className={bucket===item.key?'is-selected':''} onClick={()=>enter(item.key)}><Archive size={16} aria-hidden="true"/><span>{item.key}</span></Button>)}</nav>
      {!bucketBusy&&!bucketError&&!buckets.length&&<p>{tr('没有可见的存储桶。','No visible buckets.')}</p>}
      {bucketNext&&<Button disabled={bucketBusy} onClick={()=>void loadBuckets(bucketNext)}>{tr('加载更多桶','Load more buckets')}</Button>}
      {bucket&&<div className="webs3-prefixes"><h2>{tr('当前目录','Current folder')}</h2>{rows.filter(row=>row.kind==='prefix').map(row=><Button variant="ghost" key={row.key} onClick={()=>enter(bucket,row.key)}><Folder size={16}/><span>{row.key.slice(prefix.length)||row.key}</span></Button>)}</div>}
      <p>{tr('目录按对象前缀展示。当前版本仅浏览。','Folders represent object prefixes. Browsing only in this version.')}</p>
    </aside>
    <section className="webs3-content">
      <div className="webs3-toolbar"><h2>{bucket||tr('对象','Objects')}</h2><Button disabled={!bucket||loading} onClick={()=>setRevision(n=>n+1)}><RefreshCw size={14}/>{tr('刷新','Refresh')}</Button></div>
      {bucket&&<form className="webs3-location" onSubmit={event=>{event.preventDefault();enter(bucket,draft);}}><Button aria-label={tr('上一级','Up')} disabled={!parents.length||loading} onClick={()=>enter(bucket,parents.at(-1)||'',true)}><ArrowUp size={14}/></Button><Input aria-label={tr('对象前缀','Object prefix')} value={draft} onChange={event=>setDraft(event.target.value)} placeholder={tr('按前缀查找对象','Find objects by prefix')}/><Button type="submit" disabled={loading}>{tr('打开','Open')}</Button></form>}
      {!bucket?<div className="webs3-empty">{tr('选择存储桶以浏览对象。','Select a bucket to browse objects.')}</div>:loading?<div className="webs3-empty" role="status">{tr('正在列举对象…','Listing objects…')}</div>:failed?<Notice tone="danger">{tr('对象列举失败，请检查权限后重试。','Could not list objects. Check permissions and retry.')}</Notice>:<>
        <div className="webs3-table"><table><thead><tr><th>{tr('名称','Name')}</th><th>{tr('大小','Size')}</th><th>{tr('修改时间','Modified')}</th></tr></thead><tbody>{rows.map(row=><tr key={`${row.kind}:${row.key}`}><td><Button variant="ghost" onClick={()=>row.kind==='prefix'?enter(bucket,row.key):setSelected(row)}>{row.kind==='prefix'?<Folder size={16}/>:<File size={16}/>}<span>{row.key.slice(prefix.length)||row.key}</span></Button></td><td>{row.kind==='prefix'?'—':`${(row.size||0).toLocaleString()} B`}</td><td>{row.modified_at?new Date(row.modified_at).toLocaleString():'—'}</td></tr>)}</tbody></table>{!rows.length&&<div className="webs3-empty">{tr('此处没有对象。','No objects here.')}</div>}</div>
        <div className="webs3-pagination"><span>{tr('第','Page')} {page+1} · {rows.length} {tr('项','items')}</span><Button disabled={page===0} onClick={()=>setPage(n=>n-1)}>{tr('上一页','Previous')}</Button><Button disabled={!next||tokens.slice(0,page+1).includes(next)} onClick={()=>{setTokens(old=>[...old.slice(0,page+1),next]);setPage(n=>n+1);}}>{tr('下一页','Next')}</Button></div>
        {selected&&<dl className="webs3-detail"><dt>Key</dt><dd>{selected.key}</dd><dt>ETag</dt><dd>{selected.etag||'—'}</dd></dl>}
      </>}
    </section>
  </div>;
}
