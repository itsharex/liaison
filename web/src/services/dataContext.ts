import {request} from '@/api/client';

export type DataWorkspaceContext = {database:string; schema:string; object_type:string; name:string};

export async function syncDataContext(token:string, data:DataWorkspaceContext) {
  const result = await request<API.Response<unknown>>(`/api/v1/webdata/sessions/${encodeURIComponent(token)}/context`, {
    method:'POST', data, preserveLoginOnUnauthorized:true, signal:AbortSignal.timeout(10000),
  });
  if (result.code !== 200) throw new Error('Context sync failed');
}
