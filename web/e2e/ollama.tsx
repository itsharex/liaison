import React from 'react';
import {createRoot} from 'react-dom/client';
import {MemoryRouter,Routes,Route} from 'react-router-dom';
import AIGateway from '../src/pages/AIGateway';
import {applyThemeOnBoot} from '../src/store/theme';
import '../src/styles/index.css';
if(!import.meta.env.DEV)throw Error('Development only');
applyThemeOnBoot();
createRoot(document.getElementById('root')!).render(<MemoryRouter initialEntries={[new URLSearchParams(location.search).has('workspace')?'/ai/accesses/1':'/ai/applications/1']}><main style={{padding:24}}><Routes><Route path="/ai/applications/:applicationId" element={<AIGateway/>}/><Route path="/ai/accesses/:proxyId" element={<AIGateway/>}/></Routes></main></MemoryRouter>);
