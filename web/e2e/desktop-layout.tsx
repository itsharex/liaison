import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import {MemoryRouter} from 'react-router-dom';
import AccessContext from '../src/components/AccessContext';
import AgentWorkspace from '../src/components/AgentWorkspace';
import {Button} from '../src/components/ui';
import {applyThemeOnBoot} from '../src/store/theme';
import {usePermissions} from '../src/store/permissions';
import {useSession} from '../src/store/session';
import '../src/styles/index.css';
import '../src/pages/WebDesktop/index.less';
if(!import.meta.env.DEV)throw Error('Development only');
applyThemeOnBoot();
usePermissions.setState({owner:useSession.getState().token,loaded:true,grants:{'ai.access.use':true}});
function Fixture(){const [open,setOpen]=useState(true);return <main style={{padding:24}}><div className="webdesktop-workspace"><header className="webdesktop-toolbar"><AccessContext name="Desktop demo" protocol="RDP" target="desktop.example:3389"/><Button onClick={()=>setOpen(!open)}>Agent</Button></header><div className="webdesktop-workbench"><div className="webdesktop-shell"><div className="webdesktop-display"><span style={{color:'#fff'}}>Remote desktop</span></div></div><AgentWorkspace docked open={open} handleId="" title="Desktop demo" protocol="Web RDP" onClose={()=>setOpen(false)}/></div></div></main>}
createRoot(document.getElementById('root')!).render(<MemoryRouter><Fixture/></MemoryRouter>);
