import {useEffect,useState} from 'react';
import {useNavigate,useParams} from 'react-router-dom';
import {getWebDataTarget,getWebDesktopTarget} from '@/services/api';
import {Button,Notice} from '@/components/ui';
import {useI18n} from '@/i18n';
import {AccessConfigurationRequired,directAccessPath} from './connection';

// Compatibility entry for old bookmarked connection-manager URLs.
export default function AccessEntry({family}:{family:'webssh'|'webdesktop'|'webdata'}){
  const {proxyId}=useParams(),navigate=useNavigate(),{tr}=useI18n();
  const [failed,setFailed]=useState(false);
  useEffect(()=>{let alive=true;setFailed(false);(async()=>{
    let type:string=family;
    if(family!=='webssh'){
      const r=family==='webdesktop'?await getWebDesktopTarget(Number(proxyId)):await getWebDataTarget(Number(proxyId));
      if(r.code!==200||!r.data)throw Error('access');type=`web${r.data.protocol}`;
    }
    const path=await directAccessPath(Number(proxyId),type);if(alive)navigate(path,{replace:true});
  })().catch(error=>{if(!alive)return;if(error instanceof AccessConfigurationRequired)navigate(`/proxy?configure=${encodeURIComponent(proxyId||'')}`,{replace:true});else setFailed(true);});return()=>{alive=false;};},[family,proxyId,navigate]);
  return failed?<Notice tone="danger">{tr('无法打开访问，请检查配置。','Unable to open access. Check its configuration.')} <Button onClick={()=>navigate('/proxy')}>{tr('返回访问','Back to access')}</Button></Notice>:<div role="status">{tr('正在打开访问…','Opening access…')}</div>;
}
