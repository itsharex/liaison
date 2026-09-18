import React, {useState} from 'react';
import {createRoot} from 'react-dom/client';
import {MemoryRouter} from 'react-router-dom';
import AgentWorkspace from '../src/components/AgentWorkspace';
import {usePermissions} from '../src/store/permissions';
import {useSession} from '../src/store/session';
import '../src/styles/index.css';
if (!import.meta.env.DEV) throw Error('Development fixture only');
useSession.getState().setToken('isolation-fixture');
usePermissions.setState({owner:'isolation-fixture',loaded:true,grants:{'ai.access.use':true}});
function Fixture() {
  const [session,setSession]=useState<string>();
  const [open,setOpen]=useState(true);
  return <><nav><button onClick={()=>setSession('session_second')}>Switch conversation</button><button onClick={()=>setOpen(!open)}>Toggle panel</button></nav>
    <AgentWorkspace open={open} docked handleId="same-handle" accessId={101} connectionId="conn_a1e09f85b641f68900fe9a11" accessSessionId={session} onSessionReady={setSession} title="Isolation fixture" protocol="WebSSH" onClose={()=>setOpen(false)}/></>;
}
createRoot(document.getElementById('root')!).render(<MemoryRouter><Fixture/></MemoryRouter>);
