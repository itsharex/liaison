// Component fixture only. API traffic is intercepted by the browser test.
import React from 'react';
import {createRoot} from 'react-dom/client';
import Files from '../src/pages/WebSSH/Files';
import {usePermissions} from '../src/store/permissions';
import {useSession} from '../src/store/session';
import {applyThemeOnBoot} from '../src/store/theme';
import '../src/styles/index.css';
import '../src/pages/WebSSH/index.less';
if(!import.meta.env.DEV)throw Error('Development fixture only');
applyThemeOnBoot();
usePermissions.setState({owner:useSession.getState().token,loaded:true,grants:{'webssh.files.upload':true}});
createRoot(document.getElementById('root')!).render(<React.StrictMode><main style={{padding:16,height:'100dvh'}}><div className="webssh-shell is-files-active" style={{height:'100%',minHeight:0,maxHeight:'none'}}><header className="webssh-toolbar" style={{display:'flex',justifyContent:'space-between'}}><strong>Development · demo</strong><div className="webssh-view-tabs"><button>Terminal</button><button aria-selected>Files</button></div></header><div className="webssh-files-pane"><Files proxyId={1} username="demo" saved onClose={()=>{document.getElementById('root')!.dataset.closed='true'}}/></div></div></main></React.StrictMode>);
