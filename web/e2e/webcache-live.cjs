// Opt-in staging acceptance. Retains a named demo access for manual review;
// removes its temporary test key and sessions. Password is read from stdin.
const {request,chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 const baseURL=process.env.E2E_BASE_URL; assert(baseURL&&process.env.E2E_EMAIL&&process.env.E2E_EDGE_ID);
 const client=await request.newContext({baseURL,ignoreHTTPSErrors:true});let token,session,browser;
 const key=`liaison-e2e-${Date.now()}`;
 const call=async(method,path,data)=>{const r=await client.fetch(path,{method,data,headers:token?{Authorization:`Bearer ${token}`}:{}});const body=await r.json();assert(r.ok()&&body.code===200,`${method} ${path}: ${r.status()} ${body.message||''}`);return body.data;};
 try {
  token=(await call('POST','/api/v1/iam/login',{email:process.env.E2E_EMAIL,password:fs.readFileSync(0,'utf8').trim()})).token;
  const apps=await call('GET','/api/v1/applications?page_size=200');
  let app=apps.applications?.find(a=>a.name==='WebMemcached Demo');
  if(!app)app=await call('POST','/api/v1/applications',{name:'WebMemcached Demo',ip:'127.0.0.1',port:11211,application_type:'memcached',edge_id:Number(process.env.E2E_EDGE_ID)});
  assert.equal(app.application_type,'memcached');
  const proxies=await call('GET','/api/v1/proxies?page_size=200');
  let proxy=proxies.proxies?.find(p=>p.name==='WebMemcached Demo');
  if(!proxy)proxy=await call('POST','/api/v1/proxies',{name:'WebMemcached Demo',application_id:app.id,access_protocol:'web',expose_public_port:false});
  const root=`/api/v1/webdata/proxies/${proxy.id}`;
  let target=await call('GET',root);
  let credential=target.credentials?.[0];
  if(!credential)credential=await call('POST',root+'/credential',{name:'Memcached',protocol:'memcached',tls_mode:'disable',save_password:false});
  session=await call('POST',root+'/session',{protocol:'memcached',tls_mode:'disable'});
  const execute=async command=>{const result=await call('POST',`/api/v1/webdata/sessions/${session.token}/execute`,{statement:JSON.stringify(command)});assert(!result.error,result.error);return result;};
  const stats=await execute({operation:'stats'});assert(stats.rows.some(row=>row.name==='version'));
  assert.deepEqual((await call('GET',`/api/v1/webdata/sessions/${session.token}/metadata`)).nodes,[]);
  const value=Buffer.from('Liaison cache acceptance\n中文').toString('base64');
  await execute({operation:'set',key,value,ttl_seconds:60});
  assert.equal((await execute({operation:'get',key})).rows[0].value,value);
  await execute({operation:'delete',key});assert.equal(((await execute({operation:'get',key})).rows||[]).length,0);
  const denied=await client.post(`/api/v1/webdata/sessions/${session.token}/execute`,{data:{statement:'{"operation":"flush_all"}'},headers:{Authorization:`Bearer ${token}`}});
  const deniedBody=await denied.json();assert(!denied.ok()||deniedBody.code!==200||deniedBody.data?.error);
  const audits=await call('GET',`/api/v1/audits/access?protocol=memcached&proxy_id=${proxy.id}&page_size=100`);
  assert(audits.items?.some(item=>item.statement_preview==='memcached set'));
  assert(!JSON.stringify(audits).includes(value),'audit must not contain cached value');
  assert(!JSON.stringify(audits).includes(key),'audit must not contain cache key');
  console.log('PASS real connector Memcached stats/set/get/delete, binary round-trip, no full catalog, forbidden operation');
  browser=await chromium.launch();const context=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000}});
  await context.addInitScript(token=>{localStorage.setItem('token',token);localStorage.setItem('liaison-locale','zh-CN');localStorage.setItem('liaison-theme-preference','dark')},token);
  const page=await context.newPage(); const sessions=new Set();
  page.on('response',async r=>{if(r.url().endsWith('/session')&&r.request().method()==='POST'&&r.ok()){const b=await r.json();if(b.data?.token)sessions.add(b.data.token)}});
  try {
   await page.goto(`${baseURL}/proxy?category=cache&access_type=webmemcached`);
   await page.getByRole('cell',{name:'WebMemcached Demo',exact:true}).first().waitFor();
   console.log('PASS deployed Cache list');
   assert.equal(await page.getByRole('columnheader',{name:'用户名',exact:true}).count(),0);
   assert.equal(await page.getByRole('columnheader',{name:'数据库',exact:true}).count(),0);
   await page.goto(`${baseURL}/webdata/${proxy.id}/connections/${credential.id}?from=%2Fproxy%3Fcategory%3Dcache`);
   await page.locator('.webdata-cache-overview[aria-busy="false"] dl').waitFor();
   assert.equal(await page.locator('.webdata-cache-overview dd').first().textContent(),String(stats.rows.find(row=>row.name==='version').value));
   await page.getByRole('button',{name:'服务统计',exact:true}).click();
   await page.getByRole('button',{name:'执行',exact:true}).click();
   await page.getByText('curr_items',{exact:true}).waitFor();
   await page.screenshot({path:'/tmp/webcache-live.png'});
   console.log(`PASS deployed WebMemcached page; review /proxy?category=cache (access ${proxy.id})`);
  }finally{await page.screenshot({path:'/tmp/webcache-live-last.png'});for(const id of sessions)await call('DELETE',`/api/v1/webdata/sessions/${id}`);}
 }finally{
  if(session){await client.post(`/api/v1/webdata/sessions/${session.token}/execute`,{data:{statement:JSON.stringify({operation:'delete',key})},headers:{Authorization:`Bearer ${token}`}});await client.delete(`/api/v1/webdata/sessions/${session.token}`,{headers:{Authorization:`Bearer ${token}`}});}
  await browser?.close();await client.dispose();
 }
})().catch(e=>{console.error(String(e.message).split('\n')[0]);process.exitCode=1});
