import {spawn} from 'node:child_process';
import {createServer} from 'node:net';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';

const root=fileURLToPath(new URL('../',import.meta.url));
function port(value,fallback){const n=Number(value||fallback);if(!Number.isInteger(n)||n<1||n>65535)throw Error('Mock ports must be integers between 1 and 65535');return n;}
const apiPort=port(process.env.MOCK_API_PORT,8081),webPort=port(process.env.MOCK_WEB_PORT,5174);
const env={...process.env,MOCK_API_PORT:String(apiPort),MOCK_APP_ORIGIN:`http://127.0.0.1:${webPort}`};
const children=[];let closing=false;
function stop(code=0){
 if(closing)return;closing=true;
 for(const child of children){try{if(process.platform==='win32')child.kill('SIGTERM');else process.kill(-child.pid,'SIGTERM')}catch(error){if(error.code!=='ESRCH')console.error(error.message)}}
 setTimeout(()=>process.exit(code),500);
}
process.on('SIGINT',()=>stop());process.on('SIGTERM',()=>stop());
async function available(port){await new Promise((resolve,reject)=>{const server=createServer();server.once('error',()=>reject(Error(`Port ${port} is in use. Stop that server or select different MOCK_API_PORT/MOCK_WEB_PORT values.`)));server.listen(port,'127.0.0.1',()=>server.close(resolve))})}
function launch(command,args){
 const child=spawn(command,args,{cwd:root,env,stdio:'inherit',detached:process.platform!=='win32'});children.push(child);
 child.once('error',error=>{console.error(error.message);stop(1)});
 child.once('exit',code=>{if(!closing){console.error(`${command} stopped (exit ${code})`);stop(code||1)}});
 return child;
}
try{
 if(process.env.APP_ENV==='production')throw Error('Mock mode is not allowed in production');
 await available(apiPort);await available(webPort);
 launch('go',['run','./backend','--mock']);
 const deadline=Date.now()+60000;let ready=false;
 while(!closing&&Date.now()<deadline){
  try{const response=await fetch(`http://127.0.0.1:${apiPort}/api/health`,{signal:AbortSignal.timeout(1000)});const health=await response.json();if(response.ok&&health.mock===true){ready=true;break}}catch{}
  await delay(250);
 }
 if(!closing){
  if(!ready)throw Error('Mock API did not become ready within 60 seconds');
  launch(process.execPath,['node_modules/vite/bin/vite.js','--mode','mock','--host','127.0.0.1','--port',String(webPort),'--strictPort']);
  console.log(`\nMock web: http://127.0.0.1:${webPort}/\nMock API: http://127.0.0.1:${apiPort}/api\nAccount: demo@dwpractice.test | code: 123456\nData is disposable. Stop both servers with Ctrl+C.\n`);
 }
}catch(error){console.error(error.message);stop(1)}
