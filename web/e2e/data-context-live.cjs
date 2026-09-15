// Real staging metadata/model acceptance. No database mutations or approvals.
// E2E_BASE_URL and E2E_TOKEN_FILE (0600 JSON {token}) are supplied by the runner.
const {request}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
const demos={mysql:'Agent Demo mysql',postgresql:'Agent Demo postgresql',mongodb:'Agent Demo mongodb',redis:'Agent Demo redis',memcached:'WebMemcached Demo',elasticsearch:'Web Elasticsearch Demo'};
const flatten=nodes=>(nodes||[]).flatMap(n=>[n,...flatten(n.children)]);
(async()=>{
 assert(process.env.E2E_BASE_URL&&process.env.E2E_TOKEN_FILE,'Explicit staging URL and token file required');
 const {token}=JSON.parse(fs.readFileSync(process.env.E2E_TOKEN_FILE,'utf8'));
 const api=await request.newContext({baseURL:process.env.E2E_BASE_URL,ignoreHTTPSErrors:true,timeout:180000,extraHTTPHeaders:{Authorization:`Bearer ${token}`}});
 const call=async(method,path,data)=>{
  const response=await api.fetch(path,{method,data}),body=await response.json();
  assert(response.ok()&&[200,201].includes(body.code),`${method} ${path.replace(/sessions\/[^/]+/g,'sessions/[id]')}: HTTP ${response.status()}, code ${body.code}`);
  return body.data;
 };
 const results=[];
 try {
  const proxies=(await call('GET','/api/v1/proxies?page_size=200')).proxies;
  for(const protocol of (process.env.E2E_PROTOCOLS||Object.keys(demos).join(',')).split(',')) {
   const proxy=proxies.find(p=>p.name===demos[protocol]);assert(proxy,`Missing ${protocol} demo`);
   assert.equal(proxy.expose_public_port,false,'Only private browser demos may be enabled');
   let connection,agent,enabled=false;
   try {
    if(proxy.status==='stopped'){await call('PUT',`/api/v1/proxies/${proxy.id}`,{status:'running'});enabled=true;}
    const target=await call('GET',`/api/v1/webdata/proxies/${proxy.id}`);
    assert.equal(target.protocol,protocol);
    const credential=target.credentials.find(c=>c.saved)||target.credentials[0];assert(credential,`Missing ${protocol} credential`);
    connection=await call('POST',`/api/v1/webdata/proxies/${proxy.id}/session`,{protocol,credential_id:credential.id});
    const root=`/api/v1/webdata/sessions/${connection.token}`;
    const metadata=await call('GET',root+'/metadata');
    const objects=flatten(metadata.nodes).filter(n=>['table','collection','index','key'].includes(n.type));
    const selections=protocol==='memcached'?[{object_type:'key',name:`liaison-context-${Date.now()}`}]:objects.slice(0,2).map(n=>({database:n.meta?.database||'',schema:n.meta?.schema||'',object_type:n.type,name:n.meta?.key||n.meta?.name||n.title}));
    assert(selections.length,`No inspectable ${protocol} demo objects`);
    selections.push({database:'',schema:'',object_type:'',name:''});
    agent=await call('POST','/api/v1/agent/sessions',{handle_id:connection.token,title:`E2E current selection ${protocol}`});
    const path='/api/v1/agent/sessions/'+agent.session.id;
    for(const selection of selections){
     await call('POST',root+'/context',selection);
     const before=await call('GET',path),start=before.messages.length,turnCount=before.turns.length;
     await call('POST',path+'/turns',{prompt:'我现在选中的是哪个对象？查看最新工作区上下文后，简短说出名称；如果没有选中对象就明确说明。不要执行查询，不读取值，不做修改。'});
     let current;
     for(let i=0;i<120;i++){
      current=await call('GET',path);
      if(current.turns.length>turnCount&&[2,3,4,5].includes(current.turns.at(-1).status))break;
      await new Promise(resolve=>setTimeout(resolve,500));
     }
     assert.equal(current.turns.at(-1)?.status,3,`${protocol} Agent must complete`);
     assert.equal(current.approvals.filter(a=>a.status===0).length,0,'Metadata question must not execute queries');
     const messages=current.messages.slice(start);
     const contexts=messages.filter(m=>m.value.role==='tool').flatMap(m=>{try{const result=JSON.parse(m.value.content);const data=result.Content||result;return data.browser_context?[data.browser_context]:[];}catch{return [];}});
     assert(contexts.some(c=>c.name===(selection.name||'')),`${protocol} must refresh browser context each turn`);
     const answer=messages.filter(m=>m.value.role==='assistant').map(m=>m.value.content||'').join('\n');
     if(selection.name)assert(answer.includes(selection.name),`${protocol} must identify selected object`);
    }
    results.push({protocol,turns:selections.length,status:'passed'});
    console.log('PASS real metadata/model selection replacement and clear:',protocol);
   } finally {
    // Do not mask a failed cleanup: a successful test must restore all state.
    try {
     if(agent){const path='/api/v1/agent/sessions/'+agent.session.id,current=await call('GET',path);await call('DELETE',path,{version:current.session.version});}
    } finally {
     try {if(connection)await call('DELETE','/api/v1/webdata/sessions/'+connection.token);}
     finally {if(enabled)await call('PUT',`/api/v1/proxies/${proxy.id}`,{status:proxy.status});}
    }
   }
  }
  console.log(JSON.stringify(results));
 }finally{await api.dispose();}
})().catch(error=>{console.error(error.message);process.exitCode=1;});
