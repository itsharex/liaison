import {getToken} from '@/store/session';
import {request, RequestError} from '@/api/client';

export type StorageContext = {bucket:string;prefix:string;key:string};
export const storageBase = (token:string) => `/api/v1/webdata/sessions/${encodeURIComponent(token)}/storage`;
export const getStorageCapabilities = (token:string,signal:AbortSignal) => request<API.Response<{can_upload:boolean;transfer_limit:number}>>(`${storageBase(token)}/capabilities`,{signal,preserveLoginOnUnauthorized:true});
export async function syncStorageContext(token:string,data:StorageContext) {
  const result=await request<API.Response<unknown>>(`${storageBase(token)}/context`,{method:'POST',data,preserveLoginOnUnauthorized:true,signal:AbortSignal.timeout(10000)});
  if(result.code!==200)throw new RequestError('Could not update workspace context',result.code);
}
export async function transferStorage(token:string,location:StorageContext,signal:AbortSignal,file?:File) {
  const headers:Record<string,string>={Authorization:`Bearer ${getToken()||''}`};
  if(file)headers['Content-Type']='application/octet-stream';
  const response=await fetch(`${storageBase(token)}/${file?'upload':'download'}?${new URLSearchParams({bucket:location.bucket,key:location.key})}`,{method:file?'POST':'GET',headers,body:file,signal});
  if(!response.ok){
    const data=await response.json().catch(()=>undefined);
    throw new RequestError('Storage transfer failed',response.status,data);
  }
  if(file)return;
  const blob=await response.blob();
  if(signal.aborted)return;
  const url=URL.createObjectURL(blob),link=document.createElement('a');
  link.href=url;link.download=location.key.split('/').at(-1)||'download';link.click();
  window.setTimeout(()=>URL.revokeObjectURL(url),1000);
}
