
docker volume rm -f $(docker volume ls -q)
docker plugin disable rasmunk/sshfs:latest
make clean
make
docker plugin enable rasmunk/sshfs:latest
