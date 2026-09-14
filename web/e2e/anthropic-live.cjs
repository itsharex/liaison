// Explicit staging only. Password is read from stdin; fixture must be behind
// the selected connector. Creates and deletes only its own app/access/keys.
const {request,chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict'),fs=require('node:fs');
(async()=>{
 const origin=process.env.E2E_BASE_URL;assert(origin&&process.env.E2E_AI_EDGE_ID&&process.env.E2E_AI_PORT);
 const req=await request.newContext({baseURL:origin,ignoreHTTPSErrors:true,timeout:30000});let token,app,proxy,browser;
 const api=async(path,method='GET',data)=>{const r=await req.fetch(path,{method,data,headers:token?{Authorization:`Bearer ${token}`}:{}});assert(r.ok(),`${method} ${path}: ${r.status()}`);return (await r.json()).data};
 try{
  token=(await api('/api/v1/iam/login','POST',{email:process.env.E2E_EMAIL,password:fs.readFileSync(0,'utf8').trim()})).token;
  app=await api('/api/v1/applications','POST',{name:'Native Messages E2E',ip:'127.0.0.1',port:Number(process.env.E2E_AI_PORT),application_type:'llm',edge_id:Number(process.env.E2E_AI_EDGE_ID)});
  await api(`/api/v1/ai/applications/${app.id}`,'PUT',{protocol:'anthropic',base_path:'/anthropic',tls:false});
  proxy=await api('/api/v1/proxies','POST',{name:'Native Messages E2E',application_id:app.id,access_protocol:'aiapi',port:0,expose_public_port:false});
  const base=`/api/v1/ai/accesses/${proxy.id}`;
  await api(base,'PUT',{enabled:true,models:{chat:'fixture-chat'},external_protocol:'openai-compatible'});
  assert((await api(base+'/workspace')).external_protocols.includes('anthropic'));
  const key=await api(base+'/keys','POST',{name:'Native E2E quota',models:['chat'],expires_in_days:1,token_limit:16});
  const call=(secret,data,headers={})=>req.post(base+'/v1/messages',{headers:{...(secret?{'x-api-key':secret}:{}),'anthropic-version':'2023-06-01',...headers},data:{model:'chat',max_tokens:32,messages:[{role:'user',content:'hello'}],...data}});
  assert.equal((await call(null,{})).status(),401);
  assert.equal((await call(key.secret,{}, {Authorization:'Bearer ambiguous'})).status(),401);
  assert.equal((await call(key.secret,{model:'forbidden'})).status(),403);
  const response=await call(key.secret,{});assert.equal(response.status(),200);const body=await response.json();assert.equal(body.type,'message');assert.equal(body.model,'chat');assert.equal(body.usage.input_tokens,7);
  assert.equal((await call(key.secret,{})).status(),429);
  const streamKey=await api(base+'/keys','POST',{name:'Native E2E stream',models:['chat'],expires_in_days:1});
  const stream=await call(streamKey.secret,{stream:true});assert.equal(stream.status(),200);const text=await stream.text();assert(text.includes('event: message_stop'));assert(!text.includes('fixture-chat'));assert(!text.includes('[DONE]'));
  await api(base+'/keys/'+streamKey.id,'DELETE');assert.equal((await call(streamKey.secret,{})).status(),401);
  browser=await chromium.launch();const ctx=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000}});await ctx.addInitScript(token=>{localStorage.setItem('token',token);localStorage.setItem('liaison-locale','zh-CN');localStorage.setItem('liaison-theme-preference','dark')},token);
  const page=await ctx.newPage();await page.goto(origin+`/ai/${proxy.id}`);await page.getByLabel('调用协议',{exact:true}).selectOption('anthropic');await page.screenshot({path:'/tmp/anthropic-live.png'});
  console.log('PASS connector-backed native JSON/SSE, alias, authentication, scope, quota, revocation and UI');
 }finally{await browser?.close();if(proxy)await api('/api/v1/proxies/'+proxy.id,'DELETE');if(app)await api('/api/v1/applications/'+app.id,'DELETE');await req.dispose()}
})().catch(e=>{console.error(e);process.exitCode=1});
