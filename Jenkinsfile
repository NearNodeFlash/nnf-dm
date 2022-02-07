// Procedure for building NNF Data Movement

@Library('dst-shared@master') _

// See https://github.hpe.com/hpe/hpc-dst-jenkins-shared-library for all
// the inputs to the dockerBuildPipeline.
// In particular: vars/dockerBuildPipeline.groovy
dockerBuildPipeline {
        repository = "cray"
        imagePrefix = "cray"
        app = "dp-nnf-dm"
        name = "dp-nnf-dm"
        description = "Near Node Flash Data Movement"
        dockerfile = "Dockerfile"
        autoJira = false
        createSDPManifest = false
        product = "rabsw"
}
