import React from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter,useLocation} from 'react-router-dom';
import Audit from '../src/pages/Audit';
import Proxy from '../src/pages/Proxy';
import {RuntimeBridge} from '../src/lib/runtime';
import {applyThemeOnBoot} from '../src/store/theme';
import '../src/styles/index.css';
if(!import.meta.env.DEV)throw Error('Development fixture only');
applyThemeOnBoot();
function Page(){const location=useLocation();return <main style={{padding:24}}>{new URLSearchParams(location.search).get('screen')==='audit'?<Audit/>:<Proxy/>}</main>}
createRoot(document.getElementById('root')!).render(<BrowserRouter><RuntimeBridge/><Page/></BrowserRouter>);
