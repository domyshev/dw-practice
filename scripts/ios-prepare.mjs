import {spawnSync} from 'node:child_process';
import {writeFileSync} from 'node:fs';
const mock=process.argv.includes('--mock');
const base=mock?'http://127.0.0.1:8081':process.env.VITE_NATIVE_API_URL;
if(!base)throw Error('Set VITE_NATIVE_API_URL=https://your-deployed-server.example before preparing a production iOS build.');
const parsed=new URL(base);
if(parsed.pathname!=='/'||parsed.username||parsed.password||parsed.search||parsed.hash||(!mock&&parsed.protocol!=='https:'))throw Error('VITE_NATIVE_API_URL must be a clean HTTPS origin.');
const env={...process.env,VITE_NATIVE_API_URL:parsed.origin,VITE_NATIVE_REQUEST_ORIGIN:mock?'http://127.0.0.1:5173':process.env.VITE_NATIVE_REQUEST_ORIGIN||parsed.origin};
function run(command,args){const result=spawnSync(command,args,{stdio:'inherit',env});if(result.status!==0)process.exit(result.status||1)}
run(process.execPath,['node_modules/typescript/bin/tsc','-b']);
run(process.execPath,['node_modules/vite/bin/vite.js','build','--mode',mock?'mock':'production']);
writeFileSync('dist/native-build.json',JSON.stringify({mode:mock?'mock':'production',apiURL:parsed.origin}));
run(process.execPath,['node_modules/@capacitor/cli/bin/capacitor','sync','ios']);
console.log(mock?'Prepared mock iPhone build. Keep the Go mock API running on 8081.':'Prepared production iPhone build. Configure your signing team in Xcode.');
