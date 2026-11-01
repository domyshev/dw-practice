import type {CapacitorConfig} from '@capacitor/cli';
const config:CapacitorConfig={
 appId:'com.domyshev.dwpractice',appName:'dw',webDir:'dist',
 ios:{contentInset:'never',preferredContentMode:'mobile'},
 plugins:{Keyboard:{resize:'body'},StatusBar:{overlaysWebView:true}}
};
export default config;
