import {spawnSync} from 'node:child_process';
import {existsSync} from 'node:fs';
const app='ios/DerivedData/Build/Products/Debug-iphonesimulator/App.app';
if(!existsSync(app))throw Error('Run npm run ios:mock (or ios:prepare), then npm run ios:build first.');
const target=process.env.IOS_SIMULATOR_UDID||'booted';
for(const args of [['simctl','install',target,app],['simctl','launch',target,'com.domyshev.dwpractice']]){
 const result=spawnSync('xcrun',args,{stdio:'inherit'});if(result.status!==0)process.exit(result.status||1);
}
