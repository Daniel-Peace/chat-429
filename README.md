# Chat 429

## Overview
Chat 429 is a simple CLI chatting app built entirely in Go using TCP sockets. It allows for some discord-like features including roles, channels, and permissions. Connecting and communication is done through a 
client-server architecture where there is a central server managing all connectiong as well as forwarding messages to the appropriate recipients.

Upon connecting, you are greeted with a login screen which let's you either sign in or create an account. 

After authenticating you are presented with a list of all available channels. If you are admin or a moderator, you may create and remove channels as well as ban users.

If at any point you are lost, you can type `/help` to bring up a manual style page with instructions for all available commands.
