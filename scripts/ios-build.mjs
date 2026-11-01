import {spawnSync} from 'node:child_process';
import {existsSync} from 'node:fs';
import {homedir} from 'node:os';
import {join} from 'node:path';
// Public package downloads must not open Keychain or read an existing netrc credential file.
if(['.netrc','_netrc'].some(name=>existsSync(join(homedir(),name))))throw Error('A netrc credential file exists. Build in Xcode using your preferred authentication settings; the anonymous CLI build will not read it.');
const env={...process.env,GIT_CONFIG_COUNT:'1',GIT_CONFIG_KEY_0:'credential.helper',GIT_CONFIG_VALUE_0:'',GIT_TERMINAL_PROMPT:'0'};
const result=spawnSync('xcodebuild',['-project','ios/App/App.xcodeproj','-scheme','App','-configuration','Debug','-sdk','iphonesimulator','-derivedDataPath','ios/DerivedData','-scmProvider','system','-packageAuthorizationProvider','netrc','CODE_SIGNING_ALLOWED=NO','build'],{env,stdio:'inherit'});
process.exit(result.status??1);
