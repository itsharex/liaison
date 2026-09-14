import React from 'react';
import {createRoot} from 'react-dom/client';
import {MemoryRouter,Routes,Route,useLocation} from 'react-router-dom';
import Proxy from '../src/pages/Proxy';
import AccessEntry from '../src/pages/Proxy/AccessEntry';
import {RuntimeBridge} from '../src/lib/runtime';
import {applyThemeOnBoot} from '../src/store/theme';
import '../src/styles/index.css';
if(!import.meta.env.DEV)throw Error('Development fixture only');
applyThemeOnBoot();
function Destination(){const location=useLocation();return <output>{location.pathname}{location.search}</output>}
createRoot(document.getElementById('root')!).render(<MemoryRouter initialEntries={[new URLSearchParams(location.search).get('entry')||'/proxy']}><RuntimeBridge/><main style={{padding:24}}><Routes><Route path="/proxy" element={<Proxy/>}/><Route path="/webssh/:proxyId" element={<AccessEntry family="webssh"/>}/><Route path="/webdesktop/:proxyId" element={<AccessEntry family="webdesktop"/>}/><Route path="/webdata/:proxyId" element={<AccessEntry family="webdata"/>}/><Route path="*" element={<Destination/>}/></Routes></main></MemoryRouter>);
