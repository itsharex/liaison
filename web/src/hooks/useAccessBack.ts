import {useCallback} from 'react';
import {useNavigate,useLocation} from 'react-router-dom';

export function accessSource(search:string,fallback:string){
 const from=new URLSearchParams(search).get('from')||'';
 return from.startsWith('/')&&!from.startsWith('//')&&!/[\\\u0000-\u0020]/.test(from)?from:fallback;
}
export function useAccessBack(fallback:string){
 const navigate=useNavigate(),location=useLocation();
 return useCallback(()=>{
  const source=accessSource(location.search,'');
  if(source&&source!==location.pathname+location.search)navigate(source);
  else if((window.history.state?.idx||0)>0)navigate(-1);
  else navigate(fallback);
 },[navigate,location.pathname,location.search,fallback]);
}
