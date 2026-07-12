# Dispatcharr Plugin Demo Checklist

- [ ] Configure Xtream mode with base URL, username, and password
- [ ] Confirm connection test succeeds in Xtream mode
- [ ] Confirm `Live TV` appears in Silo
- [ ] Confirm channel search works
- [ ] Confirm live playback works
- [ ] Confirm guide data renders
- [ ] Simulate upstream outage and confirm stale metadata remains visible while health shows an error
- [ ] Switch to M3U/XMLTV mode and confirm reset warning appears
- [ ] Confirm the rebuilt `Live TV` source keeps the same top-level source identity
- [ ] Confirm M3U/XMLTV fallback mode loads channels and guide data successfully
