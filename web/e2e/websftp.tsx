import React from 'react';
import {createRoot} from 'react-dom/client';
import {MemoryRouter,Routes,Route,useLocation} from 'react-router-dom';
import WebSFTP from '../src/pages/WebSFTP';
import {usePermissions} from '../src/store/permissions';
import {useSession} from '../src/store/session';
import {applyThemeOnBoot} from '../src/store/theme';
import '../src/styles/index.css';
if(!import.meta.env.DEV)throw Error('Development fixture only');
applyThemeOnBoot();
usePermissions.setState({owner:useSession.getState().token,loaded:true,grants:{'webssh.files.read':true,'webssh.files.upload':true}});
function Destination(){const l=useLocation();return <output>{l.pathname}{l.search}</output>}
createRoot(document.getElementById('root')!).render(<React.StrictMode><MemoryRouter initialEntries={['/websftp/1']}><main style={{padding:24}}><Routes><Route path="/websftp/:proxyId" element={<WebSFTP/>}/><Route path="*" element={<Destination/>}/></Routes></main></MemoryRouter></React.StrictMode>);
